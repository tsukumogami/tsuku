// Package project provides per-directory tool configuration for tsuku.
// A .tsuku.toml file in a project directory declares which tools and versions
// the project requires. LoadProjectConfig discovers the nearest config by
// walking parent directories, stopping at $HOME or any ceiling path.
package project

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ConfigFileName is the project configuration file name.
const ConfigFileName = ".tsuku.toml"

// MaxTools is the upper bound on tools in a single config file.
// Prevents resource exhaustion from maliciously large configs.
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
	// Consumers MUST render these to stderr. Not stdout: tsuku hook-env writes
	// activation output to stdout and the shell hook evaluates it, so a
	// diagnostic there would be executed rather than read -- and these messages
	// quote the offending key, which is attacker-controlled. Use
	// FprintDiagnostics rather than printing them ad hoc.
	Diagnostics []string
}

// FprintDiagnostics writes each diagnostic to w, one per line, prefixed with
// the config's path so the reader knows which file to edit.
//
// It exists so the stderr rule has one implementation rather than five, and so
// that a consumer added later inherits it. Passing os.Stdout here would defeat
// the point; callers pass os.Stderr.
func (r *ConfigResult) FprintDiagnostics(w io.Writer) {
	if r == nil {
		return
	}
	for _, d := range r.Diagnostics {
		fmt.Fprintf(w, "%s: %s\n", r.Path, d)
	}
}

// LoadProjectConfig finds the nearest .tsuku.toml by walking up from startDir.
// Returns nil if no config file is found. Returns an error only if a config
// file exists but cannot be parsed or exceeds MaxTools.
//
// startDir is resolved via filepath.EvalSymlinks before traversal to prevent
// symlink-based misdirection. Traversal stops at $HOME unconditionally;
// TSUKU_CEILING_PATHS (colon-separated) adds additional ceilings but cannot
// remove the $HOME boundary.
func LoadProjectConfig(startDir string) (*ConfigResult, error) {
	resolved, err := filepath.EvalSymlinks(startDir)
	if err != nil {
		return nil, fmt.Errorf("resolving symlinks for %s: %w", startDir, err)
	}

	ceilings := buildCeilings()

	dir := filepath.Clean(resolved)
	for {
		if isCeiling(dir, ceilings) {
			return nil, nil
		}

		configPath := filepath.Join(dir, ConfigFileName)
		if _, err := os.Stat(configPath); err == nil {
			cfg, diags, parseErr := parseConfigFile(configPath)
			if parseErr != nil {
				return nil, parseErr
			}
			return &ConfigResult{
				Config:      cfg,
				Path:        configPath,
				Dir:         dir,
				Diagnostics: diags,
			}, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root.
			return nil, nil
		}
		dir = parent
	}
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
func buildCeilings() map[string]struct{} {
	ceilings := make(map[string]struct{})

	// $HOME is unconditional.
	if home, err := os.UserHomeDir(); err == nil {
		ceilings[filepath.Clean(home)] = struct{}{}
	}

	// Additional ceilings from environment.
	if env := os.Getenv(EnvCeilingPaths); env != "" {
		for p := range strings.SplitSeq(env, ":") {
			p = strings.TrimSpace(p)
			if p != "" {
				ceilings[filepath.Clean(p)] = struct{}{}
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

// parseConfigFile reads and validates a .tsuku.toml file.
//
// It returns diagnostics alongside the config: things wrong with the file that
// do not justify refusing to load it. An error is reserved for the case where
// nothing about the file's contents is known -- it could not be read, or it is
// not TOML -- because there is then no per-entry judgement to make.
func parseConfigFile(path string) (*ProjectConfig, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg ProjectConfig
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if len(cfg.Tools) > MaxTools {
		return nil, nil, fmt.Errorf("parsing %s: declares %d tools, maximum is %d", path, len(cfg.Tools), MaxTools)
	}

	var diags []string

	// A key tsuku does not understand is dropped by the decoder without a
	// word. That is how a typo'd section name becomes "my tools stopped
	// working" with nothing to go on.
	for _, key := range md.Undecoded() {
		diags = append(diags, fmt.Sprintf("ignoring unrecognized key %q", key.String()))
	}

	// `tools = "something"` parses cleanly and yields no tools at all, so the
	// file looks fine and activates nothing. Worth saying out loud.
	if len(cfg.Tools) == 0 {
		diags = append(diags, "no tools declared (a [tools] table is expected)")
	}

	return &cfg, diags, nil
}
