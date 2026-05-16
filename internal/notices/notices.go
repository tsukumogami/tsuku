package notices

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Kind constants classify what produced a notice.
// KindUpdateResult is the zero value and represents all notices written before
// the Kind field was introduced. KindAutoApplyResult identifies notices written
// by the background auto-apply subprocess. KindVersionFallback and
// KindShellInitChange are single-view notices removed after display.
const (
	KindUpdateResult    = ""
	KindAutoApplyResult = "auto_apply_result"
	KindVersionFallback = "version_fallback"
	KindShellInitChange = "shell_init_change"
)

// Notice represents a failed auto-update for a single tool.
// Stored at $TSUKU_HOME/notices/<toolname>.json.
type Notice struct {
	Tool             string    `json:"tool"`
	AttemptedVersion string    `json:"attempted_version"`
	Error            string    `json:"error"`
	Timestamp        time.Time `json:"timestamp"`
	Shown            bool      `json:"shown"`
	// ConsecutiveFailures tracks how many times in a row this tool has failed.
	// Notices with fewer than 3 consecutive failures are suppressed (Shown=true).
	// Backward compatible: old files without this field default to 0.
	ConsecutiveFailures int `json:"consecutive_failures,omitempty"`
	// Kind classifies what produced this notice. Empty string (KindUpdateResult)
	// is the zero value for backward compatibility: existing notice files on disk
	// that have no "kind" key deserialize with Kind == "".
	Kind string `json:"kind,omitempty"`
	// Messages holds structured warning lines accumulated during an install run.
	// Backward compatible: old files without this field deserialize with nil.
	Messages []string `json:"messages,omitempty"`
	// Verb identifies which lifecycle operation produced this notice
	// ("install", "update", "rollback", "remove"). Drives per-verb
	// rendering in internal/updates/notify.go. Backward compatible:
	// old files without this field deserialize with "" and the
	// renderer falls back to the legacy "updated to" phrasing.
	Verb string `json:"verb,omitempty"`
}

// Verb constants name the lifecycle operations a Notice can describe.
// They double as the rendered verb in user-facing messages, so the
// renderer reads Notice.Verb directly to select phrasing.
const (
	VerbInstall  = "install"
	VerbUpdate   = "update"
	VerbRollback = "rollback"
	VerbRemove   = "remove"
)

// ReadNotice reads a single tool's notice file. Exported for use by apply.go.
// Returns (nil, nil) if the file does not exist.
func ReadNotice(noticesDir, toolName string) (*Notice, error) {
	return readNotice(noticesDir, toolName)
}

// LibraryNoticePrefix is the leading sentinel used in library notice
// filenames. Library notices are stored as
// $TSUKU_HOME/notices/lib--<library>.json so they do not collide with
// tool notices when a tool and library share a name. Tool names cannot
// contain the "--" sequence after validation in WriteNotice, so the
// prefix is unambiguous: any file whose stem starts with lib-- is a
// library notice, and any stem that does not is a tool notice.
const LibraryNoticePrefix = "lib--"

// WriteNotice atomically writes a notice to the notices directory.
// A new failure overwrites the previous notice for the same tool.
// Creates the directory with os.MkdirAll if it does not exist.
// Returns an error if notice.Tool contains path separators, equals "..",
// or otherwise fails the notice-name validation (see isValidNoticeName).
//
// Library notices set notice.Tool to "lib--<library>"; the validation
// accepts the leading lib-- prefix as a single sentinel and forbids any
// further "--" sequence inside the name.
func WriteNotice(noticesDir string, notice *Notice) error {
	if err := validateNoticeName(notice.Tool); err != nil {
		return err
	}
	if err := os.MkdirAll(noticesDir, 0755); err != nil {
		return fmt.Errorf("create notices directory: %w", err)
	}

	data, err := json.MarshalIndent(notice, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal notice for %s: %w", notice.Tool, err)
	}

	path := filepath.Join(noticesDir, notice.Tool+".json")
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp notice for %s: %w", notice.Tool, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename notice for %s: %w", notice.Tool, err)
	}
	return nil
}

// ReadAllNotices scans the notices directory and returns all valid notices.
// Skips dotfiles, non-JSON files, and files that fail to parse.
// Returns (nil, nil) when the directory does not exist.
func ReadAllNotices(noticesDir string) ([]Notice, error) {
	entries, err := os.ReadDir(noticesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read notices directory: %w", err)
	}

	var results []Notice
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		toolName := strings.TrimSuffix(e.Name(), ".json")
		notice, err := readNotice(noticesDir, toolName)
		if err != nil || notice == nil {
			continue
		}
		results = append(results, *notice)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.Before(results[j].Timestamp)
	})

	return results, nil
}

// ReadUnshownNotices returns only notices where Shown == false.
func ReadUnshownNotices(noticesDir string) ([]Notice, error) {
	all, err := ReadAllNotices(noticesDir)
	if err != nil {
		return nil, err
	}

	var unshown []Notice
	for _, n := range all {
		if !n.Shown {
			unshown = append(unshown, n)
		}
	}
	return unshown, nil
}

// MarkShown reads the notice for toolName, sets Shown = true, and rewrites the file.
// No-op (returns nil) if the file does not exist.
func MarkShown(noticesDir, toolName string) error {
	notice, err := readNotice(noticesDir, toolName)
	if err != nil || notice == nil {
		return nil
	}
	notice.Shown = true
	return WriteNotice(noticesDir, notice)
}

// RemoveNotice deletes a tool's notice file.
// Returns nil if the file does not exist.
// Rejects names with path separators or equal to ".." to match
// WriteNotice's validation; defense-in-depth against future call sites.
// Library notice names (lib--<library>) are accepted symmetrically.
func RemoveNotice(noticesDir, toolName string) error {
	if err := validateNoticeName(toolName); err != nil {
		return err
	}
	path := filepath.Join(noticesDir, toolName+".json")
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove notice for %s: %w", toolName, err)
	}
	return nil
}

// validateNoticeName enforces the on-disk notice filename contract:
//
//   - empty names are rejected (nothing to write)
//   - path-separator chars and ".." are rejected (defense against
//     directory traversal — same surface WriteNotice has always guarded)
//   - the "--" sequence is rejected anywhere except as the leading
//     LibraryNoticePrefix sentinel
//
// Tool names cannot contain "--" because tsuku's tool naming convention
// is single-hyphen kebab-case ("aws-cli", "gh", "cargo-audit"); the
// validation tightens that into a checked invariant so a future
// caller cannot accidentally produce a name like "foo--bar.json" that
// looks like a library notice to the renderer.
//
// Library names (the substring after "lib--") follow the same single-
// hyphen rule and must not themselves contain "--", path separators,
// or "..".
func validateNoticeName(name string) error {
	if name == "" {
		return fmt.Errorf("invalid notice name: must not be empty")
	}
	if name == ".." || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid notice name %q: must not contain path separators", name)
	}
	if strings.HasPrefix(name, LibraryNoticePrefix) {
		lib := strings.TrimPrefix(name, LibraryNoticePrefix)
		if lib == "" {
			return fmt.Errorf("invalid notice name %q: library name after %q prefix must not be empty", name, LibraryNoticePrefix)
		}
		if strings.Contains(lib, "--") {
			return fmt.Errorf("invalid notice name %q: library name must not contain %q outside the leading prefix", name, "--")
		}
		if lib == ".." || strings.Contains(lib, "/") || strings.Contains(lib, "\\") {
			return fmt.Errorf("invalid notice name %q: library segment must not contain path separators", name)
		}
		return nil
	}
	if strings.Contains(name, "--") {
		return fmt.Errorf("invalid notice name %q: %q is reserved for the library prefix", name, "--")
	}
	return nil
}

// NoticesDir returns the notices directory path for a given config home.
func NoticesDir(homeDir string) string {
	return filepath.Join(homeDir, "notices")
}

// readNotice reads a single tool's notice file.
func readNotice(noticesDir, toolName string) (*Notice, error) {
	path := filepath.Join(noticesDir, toolName+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read notice for %s: %w", toolName, err)
	}

	var notice Notice
	if err := json.Unmarshal(data, &notice); err != nil {
		return nil, fmt.Errorf("parse notice for %s: %w", toolName, err)
	}
	return &notice, nil
}
