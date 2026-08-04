package shellenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ManagedBinaries names the binaries one managed tool puts on PATH.
type ManagedBinaries struct {
	// Tool is the tsuku tool name, used to attribute a shadowed binary back to
	// something the user can act on. It is not always the binary name --
	// ripgrep provides rg.
	Tool string

	// Binaries are the binary names the tool's active version provides.
	Binaries []string
}

// ShadowedBinary reports one binary that resolves to something other than
// tsuku's copy.
type ShadowedBinary struct {
	// Tool is the managed tool that provides Binary.
	Tool string

	// Binary is the name as it appears on PATH.
	Binary string

	// Resolved is the path a shell would actually run.
	Resolved string

	// Expected is tsuku's own copy, the one Resolved is winning against.
	Expected string
}

// PrecedenceInput carries everything CheckPrecedence needs. PathDirs is passed
// in rather than read from the environment so the check can be exercised
// without mutating the process's own PATH.
type PrecedenceInput struct {
	// TsukuHome is $TSUKU_HOME.
	TsukuHome string

	// PathDirs is PATH already split, in order. Callers pass
	// filepath.SplitList(os.Getenv("PATH")).
	PathDirs []string

	// Tools are the visible managed tools to check. Hidden tools must not
	// appear here: they are installed with CreateSymlinks false, so they have
	// no entry in tools/current and every one of them would resolve to a
	// system copy on a healthy machine. install.BuildManagedBinaries builds
	// this list with that exclusion applied.
	Tools []ManagedBinaries
}

// CheckPrecedence reports the managed binaries that something earlier on PATH
// is shadowing.
//
// doctor's other PATH checks ask whether tsuku's directories are present. This
// one asks whether they win, which is a different question: a directory can sit
// on PATH behind a competing installer's prefix and never be reached. The
// invariant is that a binary tsuku put on PATH is the one that answers.
//
// A binary is shadowed when the first executable of that name along PATH lies
// outside $TSUKU_HOME entirely. The question is whether a competing installer's
// prefix is winning, so anywhere under $TSUKU_HOME counts as tsuku's and none of
// them is a conflict. There are more of those than the two the env file puts on
// PATH: $TSUKU_HOME/bin holds shims and comes first, $TSUKU_HOME/tools/current
// holds the version symlinks, and a tool's own $TSUKU_HOME/tools/<name>-<version>/bin
// lands on PATH whenever a recipe exports it or tsuku runs the tool with its
// runtime dependencies. Comparing against tools/current alone reports all three
// of the others, which on a real machine means a warning per shim and per
// exported install directory.
//
// A binary that resolves nowhere is not reported. That is a broken install
// rather than a precedence problem, and the tools/current and bin checks
// already speak to it.
func CheckPrecedence(in PrecedenceInput) []ShadowedBinary {
	home := absHome(in.TsukuHome)
	if home == "" {
		return nil
	}

	// Resolve once per binary name. Two tools that both provide a binary of the
	// same name would otherwise walk PATH twice and report it twice.
	seen := make(map[string]bool)

	var shadowed []ShadowedBinary
	for _, tool := range in.Tools {
		for _, binary := range tool.Binaries {
			if binary == "" || seen[binary] {
				continue
			}
			seen[binary] = true

			resolved := resolveOnPath(in.PathDirs, binary)
			if resolved == "" {
				continue
			}
			if withinHome(home, resolved) {
				continue
			}

			expected := ownedCopy(in.TsukuHome, binary)
			if expected == "" {
				// tsuku records the binary but has no copy of it on disk. The
				// name resolving elsewhere is then correct, not a shadow.
				continue
			}

			shadowed = append(shadowed, ShadowedBinary{
				Tool:     tool.Tool,
				Binary:   binary,
				Resolved: resolved,
				Expected: expected,
			})
		}
	}

	return shadowed
}

// absHome cleans and absolutizes $TSUKU_HOME. Absolutizing matters: a relative
// home compared against the absolute path a PATH walk produces matches nothing,
// which would turn every managed binary into a reported shadow. An empty home is
// rejected rather than absolutized, since it would otherwise resolve to whatever
// directory doctor happened to run in.
func absHome(tsukuHome string) string {
	if tsukuHome == "" {
		return ""
	}
	home, err := filepath.Abs(tsukuHome)
	if err != nil {
		return ""
	}
	return home
}

// withinHome reports whether path lies inside home. Both are already cleaned
// absolute paths.
//
// The trailing separator is what keeps a sibling from matching: without it
// "/home/u/.tsuku-backup/bin/rg" reads as living under "/home/u/.tsuku". Same
// reason ValidateSymlinkTarget adds one.
func withinHome(home, path string) bool {
	return strings.HasPrefix(path, home+string(filepath.Separator))
}

// ownedCopy returns tsuku's own path for a binary, preferring bin/ because PATH
// order does. An empty result means tsuku has no copy on disk.
func ownedCopy(tsukuHome, binary string) string {
	for _, dir := range []string{
		filepath.Join(tsukuHome, "bin"),
		filepath.Join(tsukuHome, "tools", "current"),
	} {
		candidate := filepath.Join(dir, binary)
		if isExecutableFile(candidate) {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return candidate
			}
			return abs
		}
	}
	return ""
}

// resolveOnPath walks dirs in order and returns the first executable named
// binary, as an absolute cleaned path. It reproduces what a shell does rather
// than calling exec.LookPath, which consults the process's own environment.
//
// An empty PATH entry means the working directory to a shell. That is not a
// place tsuku can reason about, and treating it as one would make the result
// depend on where doctor was run, so it is skipped.
func resolveOnPath(dirs []string, binary string) string {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, binary)
		if !isExecutableFile(candidate) {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			return candidate
		}
		return abs
	}
	return ""
}

// isExecutableFile reports whether path is something a shell would run: a
// regular file (following symlinks, since tools/current is all symlinks) with
// an execute bit. On Windows the mode bits carry no such signal, so existence
// is the whole test.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0111 != 0
}
