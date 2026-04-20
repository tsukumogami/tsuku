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
// by the background auto-apply subprocess.
const (
	KindUpdateResult    = ""
	KindAutoApplyResult = "auto_apply_result"
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
}

// ReadNotice reads a single tool's notice file. Exported for use by apply.go.
// Returns (nil, nil) if the file does not exist.
func ReadNotice(noticesDir, toolName string) (*Notice, error) {
	return readNotice(noticesDir, toolName)
}

// WriteNotice atomically writes a notice to the notices directory.
// A new failure overwrites the previous notice for the same tool.
// Creates the directory with os.MkdirAll if it does not exist.
func WriteNotice(noticesDir string, notice *Notice) error {
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
func RemoveNotice(noticesDir, toolName string) error {
	path := filepath.Join(noticesDir, toolName+".json")
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove notice for %s: %w", toolName, err)
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
