// Package project provides per-directory tool configuration for tsuku.
// A .tsuku.toml file in a project directory declares which tools and versions
// the project requires. LoadProjectConfig discovers the nearest config by
// walking parent directories, stopping at $HOME or any ceiling path.
package project

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ConfigFileName is the project configuration file name.
const ConfigFileName = ".tsuku.toml"

// MaxTools is the upper bound on tools in a single config file.
//
// It is a post-decode count, not an input bound: the check runs after
// toml.Decode has already parsed the whole file into memory, so it caps what
// downstream code will iterate, not what the decoder will allocate. It is not a
// defense against a maliciously large config -- bounding that would mean
// limiting the input before decoding it.
const MaxTools = 256

// EnvCeilingPaths is the environment variable for additional ceiling directories.
const EnvCeilingPaths = "TSUKU_CEILING_PATHS"

// ProjectConfig represents per-directory tool requirements declared in
// a .tsuku.toml file.
type ProjectConfig struct {
	Tools map[string]ToolRequirement `toml:"tools"`
}

// ToolRequirement specifies a tool and optional version constraint.
// It accepts both string shorthand (node = "20.16.0") and inline table
// form (python = { version = "3.12" }) via a custom UnmarshalTOML method.
type ToolRequirement struct {
	Version string `toml:"version"`
}

// UnmarshalTOML implements the BurntSushi/toml Unmarshaler interface.
// It handles the string-or-table duality:
//
//	node = "20.16.0"          -> ToolRequirement{Version: "20.16.0"}
//	python = {version="3.12"} -> ToolRequirement{Version: "3.12"}
func (t *ToolRequirement) UnmarshalTOML(data any) error {
	switch v := data.(type) {
	case string:
		t.Version = v
		return nil
	case map[string]any:
		if ver, ok := v["version"]; ok {
			if s, ok := ver.(string); ok {
				t.Version = s
				return nil
			}
			return fmt.Errorf("version field must be a string, got %T", ver)
		}
		// Table without version field -- version defaults to empty.
		return nil
	default:
		return fmt.Errorf("tool requirement must be a version string or a table, got %T", data)
	}
}

// ConfigResult holds a parsed config and the path where it was found.
type ConfigResult struct {
	Config *ProjectConfig
	Path   string // absolute path to the .tsuku.toml file
	Dir    string // directory containing the config file

	// Diagnostics are things wrong with the file that did not stop it loading:
	// a declaration that was refused, a section that was ignored. They are
	// carried here rather than returned as an error because an error aborts
	// the whole load, and most of these concern one entry.
	//
	// Render them with FprintDiagnostics, which owns the stderr rule.
	Diagnostics []string
}

// FprintDiagnostics writes each diagnostic to w, one per line, prefixed with
// the config's path so the reader knows which file to edit.
//
// Callers pass os.Stderr, and never os.Stdout: `tsuku hook-env` writes
// activation output to stdout for the shell hook to evaluate, so a diagnostic
// sent there would be executed rather than read -- and these messages quote a
// key that came from the config file. The rule lives here, in one place, so a
// consumer added later inherits it instead of rediscovering it.
func (r *ConfigResult) FprintDiagnostics(w io.Writer) {
	if r == nil {
		return
	}
	for _, d := range r.Diagnostics {
		// %q on the path for the same reason the diagnostics themselves quote
		// the offending key: this is a path from a cloned repository, and a
		// control character in it would otherwise reach the terminal raw.
		fmt.Fprintf(w, "%q: %s\n", r.Path, d)
	}
}

// LoadProjectConfig finds the nearest .tsuku.toml by walking up from startDir.
// Returns nil if no config file is found. Returns an error only if a config
// file exists but cannot be read or parsed, or exceeds MaxTools.
//
// startDir is resolved via symlinks before traversal to prevent symlink-based
// misdirection. Traversal stops at $HOME unconditionally; TSUKU_CEILING_PATHS
// (colon-separated) adds additional ceilings but cannot remove the $HOME
// boundary. Both are compared resolved, as the walk is.
func LoadProjectConfig(startDir string) (*ConfigResult, error) {
	return LoadProjectConfigIn(OSDiscoveryEnv(), startDir)
}

// LoadProjectConfigIn is LoadProjectConfig with its filesystem and identity
// access supplied explicitly.
//
// The one-argument form above is what production calls; this form exists
// because the cases that decide the rule -- another user's files, files owned
// by root, a directory only an administrator could have created -- have to be
// reachable from a test running as an unprivileged user with no second account,
// and from the command package as well as from here.
func LoadProjectConfigIn(env DiscoveryEnv, startDir string) (*ConfigResult, error) {
	resolved, err := env.EvalSymlinks(startDir)
	if err != nil {
		return nil, fmt.Errorf("resolving symlinks for %s: %w", startDir, err)
	}

	ceilings := buildCeilings(env)

	for _, dir := range env.Ancestors(resolved) {
		if isCeiling(dir, ceilings) {
			return nil, nil
		}

		configPath := filepath.Join(dir, ConfigFileName)
		meta, statErr := env.Lstat(configPath)
		if statErr != nil {
			if errors.Is(statErr, fs.ErrNotExist) {
				continue
			}
			// The directory cannot be examined. End the walk here rather than
			// treating it as one holding no config and continuing past it:
			// nothing was found, so there is nothing to name, and stopping is
			// the fail-closed direction. This is the one place unreadable
			// metadata ends the walk quietly instead of reporting.
			return nil, nil
		}

		cfg, diags, readErr := readConfig(env, configPath, meta)
		if readErr != nil {
			return nil, &ParseError{Dir: dir, Path: configPath, Err: readErr}
		}
		return &ConfigResult{
			Config:      cfg,
			Path:        configPath,
			Dir:         dir,
			Diagnostics: diags,
		}, nil
	}
	return nil, nil
}

// readConfig opens the entry and decodes it, deciding and reading on one
// object rather than on a path.
//
// The open comes first and the check is made on the descriptor: a stat-then-read
// pair leaves a window in which the file the decision was made about is not the
// file whose bytes are parsed.
func readConfig(env DiscoveryEnv, path string, meta FileMeta) (*ProjectConfig, []string, error) {
	f, err := env.OpenNoFollow(path)
	if errors.Is(err, ErrIsSymlink) {
		return readThroughSymlink(env, path)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading: %w", err)
	}
	return decodeOpened(f, meta)
}

// readThroughSymlink handles the entry that is itself a symlink.
//
// Opening without following fails on a link rather than succeeding, so this is
// reached by that failure. The target's directory components are resolved, the
// way the start directory already is; a chain of more than one link is refused
// rather than walked, because each additional hop is another object nothing
// checked.
func readThroughSymlink(env DiscoveryEnv, path string) (*ProjectConfig, []string, error) {
	raw, err := env.Readlink(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading link: %w", err)
	}
	target := raw
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}

	immediate, err := env.Lstat(target)
	if err != nil {
		return nil, nil, fmt.Errorf("reading link target: %w", err)
	}
	if immediate.IsSymlink() {
		return nil, nil, errors.New("is a symlink to another symlink")
	}

	resolved, err := env.EvalSymlinks(target)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving link target: %w", err)
	}
	targetMeta, err := env.Lstat(resolved)
	if err != nil {
		return nil, nil, fmt.Errorf("reading link target: %w", err)
	}

	f, err := env.OpenNoFollow(resolved)
	if err != nil {
		return nil, nil, fmt.Errorf("reading: %w", err)
	}
	return decodeOpened(f, targetMeta)
}

// decodeOpened checks the handle against the metadata the decision was made on
// and decodes its bytes. It closes the handle.
func decodeOpened(f File, expect FileMeta) (*ProjectConfig, []string, error) {
	defer f.Close()

	opened, err := f.Meta()
	if err != nil {
		return nil, nil, fmt.Errorf("reading: %w", err)
	}
	if opened.Dev != expect.Dev || opened.Ino != expect.Ino {
		return nil, nil, errors.New("changed between the check and the read")
	}
	if !opened.IsRegular() {
		// A named pipe here blocks every shell prompt; a directory or a device
		// node is not a config either. Judged on the object whose bytes would
		// be parsed, so a symlink to an ordinary file still loads.
		return nil, nil, errors.New("is not a regular file")
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, fmt.Errorf("reading: %w", err)
	}
	return decodeConfig(data)
}

// FindProjectDir returns the directory containing the nearest .tsuku.toml,
// or "" if none found. Parse errors are silently ignored; use LoadProjectConfig
// when error handling is needed.
func FindProjectDir(startDir string) string {
	result, err := LoadProjectConfig(startDir)
	if err != nil || result == nil {
		return ""
	}
	return result.Dir
}

// buildCeilings returns the set of directories that stop traversal.
// $HOME is always included. TSUKU_CEILING_PATHS adds extras.
//
// Every entry is resolved through symlinks before it enters the set, because
// the walk it is compared against is resolved. Comparing one resolved path
// against one unresolved one is a test that never matches: a symlinked $HOME,
// or a ceiling naming a path through a link, silently stops nothing.
//
// An entry that cannot be resolved is kept as written. It names a directory
// that does not exist, or one the process cannot reach, and either way it stops
// nothing -- keeping it costs a string comparison and dropping it would differ
// only in hiding a typo.
func buildCeilings(env DiscoveryEnv) map[string]struct{} {
	ceilings := make(map[string]struct{})

	add := func(p string) {
		p = filepath.Clean(p)
		if resolved, err := env.EvalSymlinks(p); err == nil {
			ceilings[resolved] = struct{}{}
			return
		}
		ceilings[p] = struct{}{}
	}

	// $HOME is unconditional.
	if home, err := os.UserHomeDir(); err == nil {
		add(home)
	}

	// Additional ceilings from environment.
	if raw := os.Getenv(EnvCeilingPaths); raw != "" {
		for p := range strings.SplitSeq(raw, ":") {
			p = strings.TrimSpace(p)
			if p != "" {
				add(p)
			}
		}
	}

	return ceilings
}

// isCeiling reports whether dir matches any ceiling path.
func isCeiling(dir string, ceilings map[string]struct{}) bool {
	_, ok := ceilings[dir]
	return ok
}

// decodeConfig decodes and validates one .tsuku.toml's bytes.
//
// It takes bytes rather than a path because the caller has already opened the
// object the decision was made on; handing this function a path would reopen it
// and reintroduce the gap the open-then-check sequence closes.
//
// It returns diagnostics alongside the config: things wrong with the file that
// do not justify refusing to load it. An error is reserved for the case where
// nothing about the file's contents is known -- it could not be read, or it is
// not TOML -- because there is then no per-entry judgement to make.
//
// Its errors are unwrapped causes; LoadProjectConfigIn wraps them in a
// ParseError carrying the directory and the path. The path is deliberately
// absent from these messages: adding it here renders it twice in the
// diagnostic, which is the duplicated-error half of the defect that work
// removed. TestHookEnv_ParseFailure asserts the path appears exactly once.
func decodeConfig(data []byte) (*ProjectConfig, []string, error) {
	var cfg ProjectConfig
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing: %w", err)
	}

	if len(cfg.Tools) > MaxTools {
		return nil, nil, fmt.Errorf("declares %d tools, maximum is %d", len(cfg.Tools), MaxTools)
	}

	var diags []string

	// The boundary. Every consumer reaches its values through this function,
	// so refusing here means none of them can be reached with an unchecked
	// value -- including a consumer written later that never learns the rule.
	kept, refusals := validateDeclarations(cfg.Tools)
	cfg.Tools = kept
	diags = append(diags, refusals...)

	// A key tsuku does not understand is dropped by the decoder without a
	// word. That is how a typo'd section name becomes "my tools stopped
	// working" with nothing to go on.
	for _, key := range md.Undecoded() {
		diags = append(diags, fmt.Sprintf("ignoring unrecognized key %q", key.String()))
	}

	// `tools = "something"` parses cleanly and yields no tools at all, so the
	// file looks fine and activates nothing. Worth saying out loud.
	if len(cfg.Tools) == 0 && len(refusals) == 0 {
		diags = append(diags, "no tools declared (a [tools] table is expected)")
	}

	return &cfg, capDiagnostics(diags), nil
}

// maxDiagnostics bounds how much one config file can print.
//
// MaxTools bounds the refusals, but nothing bounded the undecoded keys, and
// those are the unbounded half: a file declaring fifty thousand unrecognized
// keys is cheap to write and costs nothing to parse. Without a cap it produced
// fifty thousand stderr lines -- and because `tsuku hook-env` runs from the
// shell prompt, that is fifty thousand lines on every prompt, in any directory
// at or below the one holding the file.
//
// This is worth stating plainly because the diagnostics are new here and the
// flood came with them. Before, an unrecognized key was dropped in silence;
// reporting it is the improvement, and reporting all of them turned a silent
// drop into a denial of service against exactly the untrusted surface the rest
// of this change hardens. A cap is what makes the improvement safe to keep.
const maxDiagnostics = 20

func capDiagnostics(diags []string) []string {
	if len(diags) <= maxDiagnostics {
		return diags
	}
	// Full slice expression: appending to a truncated slice would otherwise
	// overwrite the first dropped entry in the backing array.
	capped := diags[:maxDiagnostics:maxDiagnostics]
	return append(capped, fmt.Sprintf("and %d more (further diagnostics from this file suppressed)",
		len(diags)-maxDiagnostics))
}
