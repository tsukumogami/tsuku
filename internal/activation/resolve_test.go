package activation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// activate runs ComputeActivation for a project whose .tsuku.toml is the given
// body, with the given versions both recorded in state and present on disk.
func activate(t *testing.T, toml string, installedTools map[string][]string) (*ActivationResult, string) {
	t.Helper()
	projectDir, cfg, installed := setupProject(t, toml, installedTools)
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	result, err := ComputeActivation(projectDir, "", "", cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected activation result, got nil")
	}
	return result, cfg.ToolsDir
}

// wantOnly asserts the whole PATH: exactly the given bin directories, in the
// given order, ahead of the base. Spelling out the expectation rather than
// rebuilding it with cfg.ToolBinDir is what lets the assertion disagree with
// the code -- an oracle built from the function under test agrees with any
// layout that function produces, including a wrong one.
func wantOnly(t *testing.T, result *ActivationResult, toolsDir string, dirs ...string) {
	t.Helper()
	parts := make([]string, 0, len(dirs)+1)
	for _, d := range dirs {
		parts = append(parts, filepath.Join(toolsDir, d, "bin"))
	}
	parts = append(parts, "/usr/bin")
	want := strings.Join(parts, ":")
	if result.PATH != want {
		t.Errorf("PATH = %q, want %q", result.PATH, want)
	}
}

// Each of the four version forms the shell-integration guide documents resolves
// to the version installation state records. Before this, only the exact form
// worked and the other three activated nothing without saying so.
func TestResolve_AllFourDocumentedForms(t *testing.T) {
	installed := map[string][]string{"jq": {"1.6.0", "1.7.0", "1.7.1", "2.0.0"}}

	cases := []struct {
		name     string
		declared string
		want     string
	}{
		{"latest", `jq = "latest"`, "jq-2.0.0"},
		{"omitted", `jq = ""`, "jq-2.0.0"},
		{"major prefix", `jq = "1"`, "jq-1.7.1"},
		{"major-minor prefix", `jq = "1.7"`, "jq-1.7.1"},
		{"exact", `jq = "1.6.0"`, "jq-1.6.0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, toolsDir := activate(t, "[tools]\n"+tc.declared+"\n", installed)
			if len(result.Unhonorable) != 0 {
				t.Fatalf("Unhonorable = %+v, want none", result.Unhonorable)
			}
			wantOnly(t, result, toolsDir, tc.want)
		})
	}
}

// The criterion that actually discriminates version ordering from string
// ordering, and deliberately the dullest case in this file.
//
// Every digit-boundary fixture below is also satisfied by sorting the versions
// as strings ascending and taking the first, because "10.0.0" sorts before
// "9.0.0" and "0.10.0" before "0.9.0". Only a pair where the newer version is
// also lexicographically later separates the two implementations.
func TestResolve_LatestPicksTheNewerOfOneAndTwo(t *testing.T) {
	result, toolsDir := activate(t, "[tools]\njq = \"latest\"\n",
		map[string][]string{"jq": {"1.0.0", "2.0.0"}})
	wantOnly(t, result, toolsDir, "jq-2.0.0")
}

func TestResolve_DigitBoundaries(t *testing.T) {
	cases := []struct {
		name      string
		declared  string
		versions  []string
		want      string
		wantNone  bool
		wantSkip  bool
		skipsWith Reason
	}{
		{name: "9 against 10", declared: "latest", versions: []string{"9.0.0", "10.0.0"}, want: "jq-10.0.0"},
		{name: "0.9 against 0.10", declared: "latest", versions: []string{"0.9.0", "0.10.0"}, want: "jq-0.10.0"},
		{
			// Dot-boundary matching: "1" must not select 10.0.0, and neither
			// version has a 1.x.y to offer.
			name: "prefix 1 selects neither", declared: "1", versions: []string{"9.0.0", "10.0.0"},
			wantNone: true, wantSkip: true, skipsWith: ReasonNoMatch,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, toolsDir := activate(t,
				"[tools]\njq = \""+tc.declared+"\"\n",
				map[string][]string{"jq": tc.versions})
			if tc.wantNone {
				wantOnly(t, result, toolsDir)
			} else {
				wantOnly(t, result, toolsDir, tc.want)
			}
			if tc.wantSkip {
				if len(result.Unhonorable) != 1 || result.Unhonorable[0].Reason != tc.skipsWith {
					t.Errorf("Unhonorable = %+v, want one entry with reason %v", result.Unhonorable, tc.skipsWith)
				}
			} else if len(result.Unhonorable) != 0 {
				t.Errorf("Unhonorable = %+v, want none", result.Unhonorable)
			}
		})
	}
}

// Selection must not depend on the order the accessor happens to return, so an
// implementation that takes the first element without sorting fails.
func TestResolve_IgnoresAccessorOrdering(t *testing.T) {
	forward := []string{"1.0.0", "2.0.0", "3.0.0"}
	reverse := []string{"3.0.0", "2.0.0", "1.0.0"}

	for _, order := range [][]string{forward, reverse} {
		result, toolsDir := activate(t, "[tools]\njq = \"latest\"\n",
			map[string][]string{"jq": order})
		wantOnly(t, result, toolsDir, "jq-3.0.0")
	}
}

func TestResolve_PrereleaseLosesToRelease(t *testing.T) {
	result, toolsDir := activate(t, "[tools]\njq = \"latest\"\n",
		map[string][]string{"jq": {"1.0.0", "1.0.0-rc.1"}})
	wantOnly(t, result, toolsDir, "jq-1.0.0")
}

func TestResolve_PrereleaseAloneStillActivates(t *testing.T) {
	result, toolsDir := activate(t, "[tools]\njq = \"latest\"\n",
		map[string][]string{"jq": {"1.0.0-rc.1"}})
	wantOnly(t, result, toolsDir, "jq-1.0.0-rc.1")
}

// Versions CompareVersions reports equal must still resolve to one answer, the
// same one every time.
//
// The PLAN states this criterion as repeated invocations in separate processes.
// Enumerating every permutation of the tied group is strictly stronger: a
// separate process samples one candidate ordering that map iteration might
// produce, and this covers all of them, including the orderings a sampled run
// would be unlucky to miss. What makes the answer unstable in the first place
// is that sort.Slice is not stable and the candidate slice is built by ranging
// a map, so ordering is exactly the axis to enumerate.
func TestResolve_TiedVersionsAreTotallyOrdered(t *testing.T) {
	groups := [][]string{
		{"1.0", "1.0.0"},
		{"1.0.0+a", "1.0.0+b"},
	}

	for _, group := range groups {
		var first string
		for i, perm := range permutations(group) {
			got := newest(perm)
			if i == 0 {
				first = got
				continue
			}
			if got != first {
				t.Errorf("newest(%v) = %q, but newest(%v) = %q; the order is not total",
					perm, got, permutations(group)[0], first)
			}
		}
	}
}

func permutations(in []string) [][]string {
	if len(in) <= 1 {
		return [][]string{append([]string(nil), in...)}
	}
	var out [][]string
	for i := range in {
		rest := make([]string, 0, len(in)-1)
		rest = append(rest, in[:i]...)
		rest = append(rest, in[i+1:]...)
		for _, p := range permutations(rest) {
			out = append(out, append([]string{in[i]}, p...))
		}
	}
	return out
}

// Candidacy is filtered before "newest" is chosen, so a declaration falls back
// to an older intact version instead of reporting on a newer broken one.
func TestResolve_FallsBackToOlderIntactVersion(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "[tools]\njq = \"latest\"\n",
		map[string][]string{"jq": {"1.0.0", "2.0.0"}})
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	// State still records 2.0.0; its files are gone.
	if err := os.RemoveAll(filepath.Join(cfg.ToolsDir, "jq-2.0.0")); err != nil {
		t.Fatal(err)
	}

	result, err := ComputeActivation(projectDir, "", "", cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantOnly(t, result, cfg.ToolsDir, "jq-1.0.0")
	if len(result.Unhonorable) != 0 {
		t.Errorf("Unhonorable = %+v, want none: an older version was activated, so there is nothing to say",
			result.Unhonorable)
	}
}

// Matching runs before the file check, so a pin nothing satisfies reports
// no-match. The other order would tell the developer to reinstall 1.6 -- a
// version their declaration never asked for.
func TestResolve_UnsatisfiedPinIsNoMatchNotMissingFiles(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "[tools]\njq = \"2\"\n",
		map[string][]string{"jq": {"1.6"}})
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	if err := os.RemoveAll(filepath.Join(cfg.ToolsDir, "jq-1.6")); err != nil {
		t.Fatal(err)
	}

	result, err := ComputeActivation(projectDir, "", "", cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Unhonorable) != 1 {
		t.Fatalf("Unhonorable = %+v, want one entry", result.Unhonorable)
	}
	if got := result.Unhonorable[0].Reason; got != ReasonNoMatch {
		t.Errorf("Reason = %v, want ReasonNoMatch", got)
	}
}

// A satisfying version whose files are gone reports missing-files and names the
// version to reinstall.
func TestResolve_MissingFilesNamesTheVersion(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "[tools]\njq = \"1\"\n",
		map[string][]string{"jq": {"1.6", "1.7"}})
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	for _, v := range []string{"1.6", "1.7"} {
		if err := os.RemoveAll(filepath.Join(cfg.ToolsDir, "jq-"+v)); err != nil {
			t.Fatal(err)
		}
	}

	result, err := ComputeActivation(projectDir, "", "", cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Unhonorable) != 1 {
		t.Fatalf("Unhonorable = %+v, want one entry", result.Unhonorable)
	}
	u := result.Unhonorable[0]
	if u.Reason != ReasonMissingFiles {
		t.Fatalf("Reason = %v, want ReasonMissingFiles", u.Reason)
	}
	if u.Version != "1.7" {
		t.Errorf("Version = %q, want %q (the newest satisfying version)", u.Version, "1.7")
	}
}

// A recorded version whose directory is missing is not activated, and neither
// is a directory with no recorded version. Installation state and the
// filesystem must both agree.
func TestResolve_DirectoryWithoutStateEntryIsNeverActivated(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "[tools]\nghost = \"latest\"\n", nil)
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	// The directory exists. Nothing records it.
	if err := os.MkdirAll(filepath.Join(cfg.ToolsDir, "ghost-1.0.0", "bin"), 0755); err != nil {
		t.Fatal(err)
	}

	result, err := ComputeActivation(projectDir, "", "", cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantOnly(t, result, cfg.ToolsDir)
	if len(result.Unhonorable) != 1 || result.Unhonorable[0].Reason != ReasonNoMatch {
		t.Errorf("Unhonorable = %+v, want one no-match entry", result.Unhonorable)
	}
}

// A stat that fails for a reason other than not-exists must also exclude the
// directory. The loop this replaces tested only os.IsNotExist and fell through
// on everything else, putting an unreadable directory on PATH.
func TestResolve_UnreadableDirectoryIsNeverActivated(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	projectDir, cfg, installed := setupProject(t, "[tools]\njq = \"1.0.0\"\n",
		map[string][]string{"jq": {"1.0.0"}})
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	// Remove search permission on the tool directory so stat of bin/ fails
	// with EACCES rather than ENOENT.
	toolDir := filepath.Join(cfg.ToolsDir, "jq-1.0.0")
	if err := os.Chmod(toolDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(toolDir, 0o755) })

	result, err := ComputeActivation(projectDir, "", "", cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantOnly(t, result, cfg.ToolsDir)
	if len(result.Unhonorable) != 1 || result.Unhonorable[0].Reason != ReasonMissingFiles {
		t.Errorf("Unhonorable = %+v, want one missing-files entry", result.Unhonorable)
	}
}

// Tool names are matched exactly. An implementation that scanned $TSUKU_HOME/tools
// for directories starting with the declared name would put git-lfs on PATH for
// a project that asked for git.
func TestResolve_ToolNamesAreNotPrefixMatched(t *testing.T) {
	result, toolsDir := activate(t, "[tools]\ngit = \"latest\"\n",
		map[string][]string{"git-lfs": {"3.5.1"}})

	wantOnly(t, result, toolsDir)
	if strings.Contains(result.PATH, "git-lfs") {
		t.Errorf("PATH = %q, must not contain a git-lfs entry", result.PATH)
	}
	if len(result.Unhonorable) != 1 || result.Unhonorable[0].Tool != "git" {
		t.Errorf("Unhonorable = %+v, want one entry for git", result.Unhonorable)
	}
}

// A legitimate org-scoped key resolves through SplitOrgKey to its bare recipe
// name, which is the path component the installer wrote.
func TestResolve_OrgScopedKeyResolvesToBareName(t *testing.T) {
	cases := []struct {
		key  string
		bare string
	}{
		{"tsukumogami/koto", "koto"},
		{"tsukumogami/registry:mytool", "mytool"},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			result, toolsDir := activate(t,
				"[tools]\n\""+tc.key+"\" = \"latest\"\n",
				map[string][]string{tc.bare: {"1.0.0"}})

			if len(result.Unhonorable) != 0 {
				t.Fatalf("Unhonorable = %+v, want none", result.Unhonorable)
			}
			wantOnly(t, result, toolsDir, tc.bare+"-1.0.0")
		})
	}
}

// A malformed declared version is bad-form, not no-match. The distinction is
// the whole point of reporting a reason: no-match sends the developer to look
// at what is installed, and the problem is the line they wrote.
func TestResolve_MalformedDeclaredVersionIsBadForm(t *testing.T) {
	for _, declared := range []string{"../evil", "1.0.0/../..", `1.0\0`, "1.0 0"} {
		t.Run(declared, func(t *testing.T) {
			result, toolsDir := activate(t,
				"[tools]\njq = '"+declared+"'\n",
				map[string][]string{"jq": {"1.0.0"}})

			wantOnly(t, result, toolsDir)
			if len(result.Unhonorable) != 1 {
				t.Fatalf("Unhonorable = %+v, want one entry", result.Unhonorable)
			}
			if got := result.Unhonorable[0].Reason; got != ReasonBadForm {
				t.Errorf("Reason = %v, want ReasonBadForm", got)
			}
			if result.Unhonorable[0].Declared != declared {
				t.Errorf("Declared = %q, want %q", result.Unhonorable[0].Declared, declared)
			}
		})
	}
}

// withinToolsDir is the guard that keeps a composed path inside
// $TSUKU_HOME/tools. It is tested directly because, with the name and version
// checks above it in place, no .tsuku.toml can currently produce an input that
// reaches it -- an end-to-end test of it would pass whether the guard were
// there or not, which is no test at all. It stays as the check that keeps the
// property true if a later change adds a candidate source or relaxes one of
// those two, and this is the test that would then already be in place.
func TestWithinToolsDir(t *testing.T) {
	root := t.TempDir()
	cfg, _ := emptyConfig(t)
	cfg.ToolsDir = filepath.Join(root, "tools")

	inside := []string{
		filepath.Join(root, "tools"),
		filepath.Join(root, "tools", "jq-1.0.0", "bin"),
		filepath.Join(root, "tools", "a", "b", "c"),
	}
	for _, p := range inside {
		if !withinToolsDir(cfg, p) {
			t.Errorf("withinToolsDir(%q) = false, want true", p)
		}
	}

	outside := []string{
		root,
		filepath.Join(root, "toolsmalicious"),
		// The sibling-prefix case the appended separator exists for.
		filepath.Join(root, "tools-malicious", "bin"),
		filepath.Join(root, "tools", "..", "evil"),
		filepath.Join(root, "tools", "jq-..", "..", "..", "etc"),
		"/etc",
	}
	for _, p := range outside {
		if withinToolsDir(cfg, p) {
			t.Errorf("withinToolsDir(%q) = true, want false", p)
		}
	}
}

// Every entry activation produces lies inside $TSUKU_HOME/tools.
//
// This is a property assertion, not a test of withinToolsDir: with the name and
// version checks in place the property holds before that guard is consulted, so
// removing the guard leaves this green. TestWithinToolsDir is what pins the
// guard. This one is here to catch a future change that produces an escaping
// entry by some route neither check covers.
func TestResolve_EveryEntryIsInsideToolsDir(t *testing.T) {
	// TOML literal strings (single quotes) so nothing here is escape-processed
	// on the way in -- a basic string would turn `a\b` into a backspace, and
	// the test would be checking a name that is not the one it meant.
	keys := []string{
		`../../../etc`,
		`..%2f..%2fetc`,
		`a\b`,
		`tsukumogami/../../evil`,
		`tsukumogami/koto`,
		`jq`,
	}

	lines := []string{"[tools]"}
	for _, k := range keys {
		lines = append(lines, "'"+k+"' = \"latest\"")
	}
	toml := strings.Join(lines, "\n") + "\n"

	result, toolsDir := activate(t, toml, map[string][]string{
		"koto": {"1.0.0"},
		"jq":   {"1.7.1"},
	})

	prefix := toolsDir + string(filepath.Separator)
	for _, entry := range strings.Split(result.PATH, ":") {
		if entry == "/usr/bin" {
			continue
		}
		if !strings.HasPrefix(entry, prefix) {
			t.Errorf("PATH entry %q escapes %q", entry, toolsDir)
		}
	}
}

// A key whose derived bare name is not a single safe path segment is rejected,
// names the offending key, and contributes nothing to PATH.
//
// The assertion is on the derived name, not on the key containing "/": an
// org-scoped key legitimately contains one, and a criterion phrased as "a key
// with a slash activates nothing" would have described the org-key bug as
// desired behavior and gone red the day it was fixed.
func TestResolve_UnsafeDerivedNameIsRejected(t *testing.T) {
	cases := []string{
		"../../../etc",
		`a\b`,
		"tsukumogami/../../evil",
		"..",
	}

	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			// A TOML literal string, so the key reaching activation is the one
			// written here. In a basic string `\b` is a backspace, and the
			// test would pass a name that is not the one it meant to reject.
			result, toolsDir := activate(t,
				"[tools]\n'"+key+"' = \"latest\"\n", nil)

			wantOnly(t, result, toolsDir)
			if len(result.Unhonorable) != 1 {
				t.Fatalf("Unhonorable = %+v, want one entry", result.Unhonorable)
			}
			u := result.Unhonorable[0]
			if u.Reason != ReasonBadForm {
				t.Errorf("Reason = %v, want ReasonBadForm", u.Reason)
			}
			// The message has to name the line the developer will go and edit,
			// which is the key as written, not the derived name.
			if u.Tool != key {
				t.Errorf("Tool = %q, want the key as written, %q", u.Tool, key)
			}
		})
	}
}

// A version recorded in installation state becomes a path component, and no
// parse-time check ever sees it: the file said "latest".
//
// "a/b" is the fixture that isolates the validator. It stays inside
// $TSUKU_HOME/tools once joined, so the containment guard would let it through,
// and the directory is created here so the stat would succeed. Only
// install.ValidateVersionString rejects it.
func TestResolve_StateDerivedVersionIsValidated(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "[tools]\njq = \"latest\"\n", nil)
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	installed.versions["jq"] = []string{"a/b"}
	if err := os.MkdirAll(filepath.Join(cfg.ToolsDir, "jq-a", "b", "bin"), 0755); err != nil {
		t.Fatal(err)
	}

	result, err := ComputeActivation(projectDir, "", "", cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantOnly(t, result, cfg.ToolsDir)
}

// The validation at the sink is never stricter than the gate that created the
// directory, so a version tsuku installed always activates. internal/version
// exports a ValidateVersionString that rejects these, which is the other half
// of why it is the wrong function to call here.
func TestResolve_VersionsTsukuInstalledAlwaysActivate(t *testing.T) {
	for _, v := range []string{"1.0.0 beta", "1.0.0~rc", "2024.01.15", "v1.2.3", "1.0.0+build.5"} {
		t.Run(v, func(t *testing.T) {
			result, toolsDir := activate(t, "[tools]\njq = \"latest\"\n",
				map[string][]string{"jq": {v}})
			if len(result.Unhonorable) != 0 {
				t.Fatalf("Unhonorable = %+v, want none for a version tsuku installed", result.Unhonorable)
			}
			wantOnly(t, result, toolsDir, "jq-"+v)
		})
	}
}

func TestResolve_ChannelPinReportsChannel(t *testing.T) {
	result, toolsDir := activate(t, "[tools]\nnode = \"@lts\"\n",
		map[string][]string{"node": {"20.16.0"}})

	wantOnly(t, result, toolsDir)
	if len(result.Unhonorable) != 1 || result.Unhonorable[0].Reason != ReasonChannel {
		t.Fatalf("Unhonorable = %+v, want one channel entry", result.Unhonorable)
	}
	if result.Unhonorable[0].Declared != "@lts" {
		t.Errorf("Declared = %q, want %q", result.Unhonorable[0].Declared, "@lts")
	}
}

// Installation state is read once per activation, however many tools are
// declared. This is the criterion the consumer-declared interface exists to
// make testable.
func TestResolve_ReadsInstallationStateOnce(t *testing.T) {
	toml := `
[tools]
jq = "latest"
node = "latest"
go = "latest"
`
	projectDir, cfg, installed := setupProject(t, toml, map[string][]string{
		"jq": {"1.7.1"}, "node": {"20.16.0"}, "go": {"1.22.0"},
	})
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	if _, err := ComputeActivation(projectDir, "", "", cfg, installed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if installed.calls != 1 {
		t.Errorf("InstalledVersionsFor called %d times, want 1", installed.calls)
	}
}

// An unreadable state file is one fact about the read, not N facts about the
// declarations, and it names only the declarations that needed it. A bad-form
// or channel declaration is classified without consulting state, so listing it
// in a message about state would be wrong.
func TestResolve_UnreadableStateNamesOnlyTheDeclarationsThatNeededIt(t *testing.T) {
	toml := `
[tools]
jq = "latest"
node = "@lts"
"../evil" = "latest"
`
	projectDir, cfg, installed := setupProject(t, toml, nil)
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	installed.err = errors.New("state.json is not valid JSON")

	result, err := ComputeActivation(projectDir, "", "", cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Unreadable == nil {
		t.Fatal("Unreadable = nil, want the read failure recorded")
	}
	if len(result.Unreadable.Tools) != 1 || result.Unreadable.Tools[0] != "jq" {
		t.Errorf("Unreadable.Tools = %v, want [jq]: node is a channel and \"../evil\" is bad-form, "+
			"and neither ever needed the read", result.Unreadable.Tools)
	}
	if result.Unreadable.Err == nil {
		t.Error("Unreadable.Err = nil, want the underlying error kept for the debug log")
	}
	wantOnly(t, result, cfg.ToolsDir)
}
