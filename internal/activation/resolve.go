package activation

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/project"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
)

// declaration is one [tools] entry whose key and version string have passed the
// checks that need no installation state.
type declaration struct {
	key      string // the key as .tsuku.toml wrote it, for messages
	bare     string // the recipe name, which is what becomes a path component
	declared string // the version string as .tsuku.toml wrote it
}

// classifyForm checks one declaration without consulting installation state.
// It returns either a usable declaration or the reason it was rejected.
//
// Splitting this out from the resolution below is what lets an unreadable
// state file name only the declarations that actually needed the read.
func classifyForm(key, declared string) (declaration, *Unhonorable) {
	badForm := func(badName bool) *Unhonorable {
		return &Unhonorable{
			Tool: key, Declared: declared, Reason: ReasonBadForm, BadName: badName,
		}
	}

	// An org-scoped key names its source and its recipe separately:
	// "tsukumogami/koto" installs to tools/koto-<version>, not to
	// tools/tsukumogami/koto-<version>. The bare name is the path component.
	//
	// Composing from the key rather than the bare name is a PATH-separator bug
	// as well as a wrong-directory one, and that half is worth keeping in view.
	// For an "owner/repo:tool" key it composes <tools>/owner/repo:tool-1.0/bin,
	// which puts a colon *inside* one PATH entry -- and the join that builds
	// PATH then splits it in two, the second half relative. The boundary
	// validates owner, repo and bare name as three separate components
	// precisely because the whole key is never a path component. (Carried from
	// the fix #2563 made at the old sink, which this rewrite replaces; the
	// reasoning is not recoverable from the code that survived.)
	_, bare, _, err := project.SplitOrgKey(key)
	if err != nil {
		return declaration{}, badForm(true)
	}

	// The name is checked before the version because it reaches filepath.Join
	// whatever the version turns out to be. recipe.IsValidRecipeName is the
	// tree's single definition of a well-formed recipe identifier, and its doc
	// comment states the question it answers -- safe to pass to path
	// construction -- which is the question this sink asks.
	//
	// This is a backstop beneath the boundary, and its reachability is
	// uncertain. Since #2563, validateDeclarations refuses a malformed key at
	// parse time, so a .tsuku.toml cannot currently deliver a name that reaches
	// here and fails -- measured, not assumed: the end-to-end version of the
	// test below stopped being able to produce a rejection, which is why it is
	// now a unit test on classifyForm.
	//
	// Deliberately not claimed: that this refuses everything the boundary
	// refuses. The two answer overlapping questions with different rules and
	// nobody has established containment in either direction. What is claimed
	// is narrower and is what a path sink needs -- the composed name is a
	// single safe segment before it is joined.
	//
	// TestClassifyForm_UnsafeDerivedNameIsRejected is what holds this up. An
	// end-to-end test cannot reach it and would be green whether or not the
	// check existed, which is the shape this chain has spent its time removing.
	if !recipe.IsValidRecipeName(bare) {
		return declaration{}, badForm(true)
	}

	if err := install.ValidateRequested(declared); err != nil {
		return declaration{}, badForm(false)
	}

	// A channel pin cannot be evaluated by matching strings against recorded
	// versions; resolving it needs the provider. Saying so is better than
	// reporting it as no-match, which would send the developer looking for a
	// version that was never the problem.
	if install.PinLevelFromRequested(declared) == install.PinChannel {
		return declaration{}, &Unhonorable{Tool: key, Declared: declared, Reason: ReasonChannel}
	}

	return declaration{key: key, bare: bare, declared: declared}, nil
}

// selectVersion picks the bin directory to put on PATH for one declaration,
// given the versions installation state records for its bare name.
//
// Candidacy is filtered before "newest" is chosen, so a declaration falls back
// to an older intact version rather than reporting on a newer broken one.
func selectVersion(d declaration, recorded []string, cfg *config.Config) (string, *Unhonorable) {
	var satisfying []string
	for _, v := range recorded {
		if !install.VersionMatchesPin(v, d.declared) {
			continue
		}
		// A state-recorded version becomes a path component, and no parse-time
		// check ever sees it: the file said "latest" and state says "26.8.1".
		//
		// install.ValidateVersionString is the one to call here, and it is a
		// choice rather than an import detail -- internal/version exports a
		// function of the same name that accepts "../../evil", because its
		// charset permits "/" for scoped npm names and "." is a legal version
		// character so ".." is never special. The two answer different
		// questions: that one asks whether a string is a plausible version
		// token, this one asks whether it is safe to compose into a path. A
		// path sink asks the second. install's is also the gate that let the
		// directory be created, so it can never refuse a version tsuku itself
		// installed.
		if err := install.ValidateVersionString(v); err != nil {
			continue
		}
		satisfying = append(satisfying, v)
	}
	if len(satisfying) == 0 {
		return "", &Unhonorable{Tool: d.key, Declared: d.declared, Reason: ReasonNoMatch}
	}

	var intact []string
	for _, v := range satisfying {
		dir := cfg.ToolBinDir(d.bare, v)
		if !withinToolsDir(cfg, dir) {
			continue
		}
		// Any stat error excludes the directory, not just not-exists. A
		// permissions failure means we cannot tell whether the tool is there,
		// and putting an unreadable directory on PATH on that basis is how the
		// old loop turned a broken install into a silent one.
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		intact = append(intact, v)
	}
	if len(intact) == 0 {
		// Something satisfied the pin, so the declaration is not what is wrong
		// -- the files are. Name the newest satisfying version, which is the
		// one the developer would reinstall.
		return "", &Unhonorable{
			Tool:     d.key,
			Declared: d.declared,
			Reason:   ReasonMissingFiles,
			Version:  newest(satisfying),
		}
	}

	return cfg.ToolBinDir(d.bare, newest(intact)), nil
}

// newest returns the highest version, breaking ties by raw string so the order
// is total.
//
// The tie-break is deterministic, not semantically correct, and the difference
// matters. Nothing can be correct once CompareVersions has said the two
// versions are equal; what matters is that identical state produces the same
// answer on every prompt and on every machine. This is not a version-ordering
// improvement waiting to be made clever.
//
// It is needed because CompareVersions returns 0 for distinct strings by three
// independent routes: compareCoreParts discards its Sscanf error, so any
// non-numeric component compares as zero; missing components pad to zero, so
// "1.0" ties "1.0.0"; and splitPrerelease strips build metadata, so "1.0.0+a"
// ties "1.0.0+b". SortVersionsDescending feeds that into sort.Slice, which Go
// documents as not stable, over a slice built by ranging a map. Without the
// tie-break the winner among tied versions is drawn fresh every prompt with
// nothing having changed -- the same command, the same state, a different
// answer, and no way for anyone to reproduce their own bug report.
func newest(versions []string) string {
	sorted := version.SortVersionsDescending(versions)
	best := sorted[0]
	for _, v := range sorted[1:] {
		// Everything the comparator ranks equal to the maximum is a prefix of
		// the sorted slice, so the first difference ends the tied group.
		if version.CompareVersions(v, sorted[0]) != 0 {
			break
		}
		if v > best {
			best = v
		}
	}
	return best
}

// withinToolsDir reports whether dir lies inside cfg.ToolsDir.
//
// The separator is appended before the prefix comparison so a sibling named
// "tools-malicious" does not match "tools"; internal/install/symlink.go guards
// its own joins the same way. Asserting containment on the composed path rather
// than inspecting the key for characters is what makes the property hold
// without anyone having enumerated every character a key must not contain.
func withinToolsDir(cfg *config.Config, dir string) bool {
	root, err := filepath.Abs(cfg.ToolsDir)
	if err != nil {
		return false
	}
	target, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	return target == root || strings.HasPrefix(target, root+string(filepath.Separator))
}
