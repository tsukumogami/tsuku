package autoinstall

import (
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/project"
)

// TestElevateWithholdsForAnUnregisteredSource asserts the decision -- the mode
// and the origin the elevation returns -- rather than whether a prompt appeared.
//
// A test on the prompt would pass for a run that reached confirm by some other
// route, and the whole point of this input is which runs reach confirm.
func TestElevateWithholdsForAnUnregisteredSource(t *testing.T) {
	for name, tc := range map[string]struct {
		mode       Mode
		origin     Origin
		declared   bool
		qualifies  bool
		wantMode   Mode
		wantOrigin Origin
	}{
		"declared, unregistered source, unset default": {
			mode: ModeConfirm, origin: OriginDefault, declared: true, qualifies: false,
			wantMode: ModeConfirm, wantOrigin: OriginDefault,
		},
		"declared, registered source, unset default": {
			mode: ModeConfirm, origin: OriginDefault, declared: true, qualifies: true,
			wantMode: ModeAuto, wantOrigin: OriginProject,
		},
		"declared, no source component, unset default": {
			mode: ModeConfirm, origin: OriginDefault, declared: true, qualifies: true,
			wantMode: ModeAuto, wantOrigin: OriginProject,
		},
		"undeclared is untouched whichever way the source falls": {
			mode: ModeConfirm, origin: OriginDefault, declared: false, qualifies: true,
			wantMode: ModeConfirm, wantOrigin: OriginDefault,
		},
	} {
		t.Run(name, func(t *testing.T) {
			mode, origin := elevate(tc.mode, tc.origin, tc.declared, tc.qualifies)
			if mode != tc.wantMode || origin != tc.wantOrigin {
				t.Errorf("elevate(%v, %v, %v, %v) = %v, %v; want %v, %v",
					tc.mode, tc.origin, tc.declared, tc.qualifies, mode, origin, tc.wantMode, tc.wantOrigin)
			}
		})
	}
}

// TestExplicitlySetModesAreUntouchedByTheSourceInput covers R21.
//
// The new input narrows the one case where a default becomes auto. It must not
// reach a mode somebody chose, whichever way the source falls -- an explicitly
// set auto stays auto, and an explicitly set confirm is not raised by a
// registered source either.
func TestExplicitlySetModesAreUntouchedByTheSourceInput(t *testing.T) {
	for _, origin := range []Origin{OriginUnset, OriginFlag, OriginEnvironment, OriginConfig} {
		for _, qualifies := range []bool{true, false} {
			for _, mode := range []Mode{ModeSuggest, ModeConfirm, ModeAuto} {
				gotMode, gotOrigin := elevate(mode, origin, true, qualifies)
				if gotMode != mode || gotOrigin != origin {
					t.Errorf("elevate(%v, %v, true, %v) changed an explicitly set mode to %v, %v",
						mode, origin, qualifies, gotMode, gotOrigin)
				}
			}
		}
	}
}

// TestNilPredicateWithholdsTheRaise pins the fail-closed default.
//
// A caller that never wired the predicate cannot tell an approved source from
// an unapproved one. Waiving the prompt on the strength of a question nobody
// answered is the failure this input exists to prevent, so an unwired predicate
// withholds -- while a key with no source component, which needs no answer,
// still qualifies.
func TestNilPredicateWithholdsTheRaise(t *testing.T) {
	r := &Runner{}

	orgScoped := &project.ProjectDeclaration{Recipe: "tool", ConfigKey: "owner/repo:tool"}
	if source, qualifies := r.declarationQualifies(orgScoped); qualifies {
		t.Errorf("an unwired predicate qualified an org-scoped key (source %q)", source)
	}

	plain := &project.ProjectDeclaration{Recipe: "tool", ConfigKey: "tool"}
	if _, qualifies := r.declarationQualifies(plain); !qualifies {
		t.Error("an unwired predicate withheld the raise for a plain key, which needs no source answer")
	}
}

// TestRegisteredSourceQualifiesWhicheverRouteRegisteredIt covers R18's second
// sentence. The predicate is the only input, so nothing about how the entry got
// into the registries can change the answer.
func TestRegisteredSourceQualifiesWhicheverRouteRegisteredIt(t *testing.T) {
	r := &Runner{SourceRegistered: func(source string) bool { return source == "owner/repo" }}

	registered := &project.ProjectDeclaration{Recipe: "tool", ConfigKey: "owner/repo:tool"}
	if _, qualifies := r.declarationQualifies(registered); !qualifies {
		t.Error("a registered source did not qualify")
	}

	other := &project.ProjectDeclaration{Recipe: "tool", ConfigKey: "someone/else:tool"}
	source, qualifies := r.declarationQualifies(other)
	if qualifies {
		t.Error("an unregistered source qualified")
	}
	if source != "someone/else" {
		t.Errorf("the withheld raise named source %q, want someone/else", source)
	}
}

// TestMembershipComparisonIsExactRatherThanCaseFolded is the criterion the plan
// review asked for.
//
// GitHub treats Owner/Repo and owner/repo as one repository, so an
// implementation that normalized case would raise for a key the user's
// configuration does not literally contain -- which is the wrong direction for
// a consent decision. Failing closed costs the honest user a prompt; failing
// open waives one nobody granted.
func TestMembershipComparisonIsExactRatherThanCaseFolded(t *testing.T) {
	// The configured entry differs from the declared key only in case.
	r := &Runner{SourceRegistered: func(source string) bool { return source == "Owner/Repo" }}

	declared := &project.ProjectDeclaration{Recipe: "tool", ConfigKey: "owner/repo:tool"}
	source, qualifies := r.declarationQualifies(declared)
	if qualifies {
		t.Fatal("a case-folded match qualified a source the configuration does not contain")
	}
	if source != "owner/repo" {
		t.Errorf("the notice names source %q, want the key's own spelling owner/repo", source)
	}
	if !strings.Contains(unregisteredSourceNotice(source), "not in your registered sources") {
		t.Error("the notice does not say the source is unregistered")
	}
}

// TestUnregisteredSourceNoticeCarriesAllThreeFacts covers R19.
//
// The third is the one a reader cannot work out for themselves and is asserted
// unconditionally: whatever the file says, the recipe comes from the user's own
// configured sources.
func TestUnregisteredSourceNoticeCarriesAllThreeFacts(t *testing.T) {
	notice := unregisteredSourceNotice("owner/repo")

	if !strings.Contains(notice, `"owner/repo"`) {
		t.Errorf("the notice does not name the source: %q", notice)
	}
	if !strings.Contains(notice, "not in your registered sources") {
		t.Errorf("the notice does not say it is unregistered: %q", notice)
	}
	if !strings.Contains(notice, "resolved through your own configured sources") {
		t.Errorf("the notice does not say where the recipe actually comes from: %q", notice)
	}
	if !strings.Contains(notice, "tsuku registry add owner/repo") {
		t.Errorf("the notice does not name a way forward: %q", notice)
	}
}

// TestCollapsedKeyIsRaised pins the limit rather than leaving it to be
// discovered.
//
// buildDeclarations collapses a tool declared both plainly and with a source
// into one declaration carrying the plain key, so a file that adds one plain
// line evades this rule. The attacker gains nothing by it -- a plain key
// already installs silently, and the source they named could never supply the
// recipe -- but the behavior should be a decision on the page rather than a
// surprise.
func TestCollapsedKeyIsRaised(t *testing.T) {
	r := &Runner{SourceRegistered: func(string) bool { return false }}

	// What buildDeclarations produces for {"tool": ..., "owner/repo:tool": ...}
	// is one declaration carrying the plain key.
	collapsed := &project.ProjectDeclaration{Recipe: "tool", ConfigKey: "tool"}
	source, qualifies := r.declarationQualifies(collapsed)
	if !qualifies {
		t.Error("a collapsed plain key was treated as naming a source")
	}
	if source != "" {
		t.Errorf("a plain key named source %q", source)
	}
}
