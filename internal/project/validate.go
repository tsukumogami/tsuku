package project

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tsukumogami/tsuku/internal/pinsafe"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// orgSegmentPattern is the character set GitHub permits in an owner or a
// repository name. Uppercase is included because GitHub allows it and real
// dependencies use it; this is the upper bound on what a legitimate
// distributed source can be, not a guess.
var orgSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validateDeclarations refuses declarations whose key or version could steer a
// path, a URL, or shell-visible text, and returns the surviving map alongside a
// diagnostic for each refusal.
//
// This is the boundary. Every consumer of a project config -- activation,
// tsuku install, tsuku shim install, the tsuku run fast path -- reaches its
// values through parseConfigFile, so validating once here means none of them
// can be reached with an unchecked value, including consumers not yet written.
// Checking at each sink instead is what produced the defect this closes: the
// codebase holds several correct guards, each wired to one consumer.
//
// Refusal is per declaration rather than per file. The security requirement is
// only that the bad value never reaches a sink; whether its siblings survive is
// a separate question, and refusing the file would disable every tool in a
// shared repository because one entry was mistyped. A file whose *contents* are
// unknown -- unreadable, or not TOML -- is still refused whole by the caller,
// because there is then no per-entry judgement available.
func validateDeclarations(tools map[string]ToolRequirement) (map[string]ToolRequirement, []string) {
	if len(tools) == 0 {
		return tools, nil
	}

	kept := make(map[string]ToolRequirement, len(tools))
	var diags []string

	for key, req := range tools {
		if err := validateKey(key); err != nil {
			diags = append(diags, fmt.Sprintf("ignoring %q: %v", key, err))
			continue
		}
		// The declared version becomes a path component too, and is as
		// unchecked as the name was. A name-only fix leaves the identical hole
		// through the other half of the same composition.
		//
		// internal/updates/apply.go removed its own version check citing this
		// one. Named here so that relaxing this rule -- permitting '+' for
		// semver build metadata, say -- is visibly a decision about two
		// packages rather than one. Its comment records a second, independent
		// reason it is safe, so this is not the only thing holding it up.
		if err := pinsafe.ValidateRequested(req.Version); err != nil {
			diags = append(diags, fmt.Sprintf("ignoring %q: %v", key, err))
			continue
		}
		kept[key] = req
	}

	return kept, diags
}

// validateKey checks both halves of a declaration key.
//
// A key is either a bare name or an org-scoped reference of the form
// owner/repo or owner/repo:tool. The two halves get different rules on purpose,
// and conflating them is the mistake this function is shaped to avoid: the bare
// name must be a single safe path segment, while an org source is a GitHub
// coordinate where uppercase is perfectly legal -- BurntSushi/toml is the TOML
// library this repository itself depends on. Applying the bare-name rule to the
// source would refuse it.
func validateKey(key string) error {
	source, bare, isOrgScoped, err := SplitOrgKey(key)
	if err != nil {
		// The splitter already rejects traversal in org-scoped keys, but its
		// only caller discards this error, which is why such a key reaches a
		// path sink today. Propagating it is half the fix: a key that yields
		// no bare name must not be treated as though it passed validation.
		return err
	}

	if isOrgScoped {
		if serr := validateOrgSource(source); serr != nil {
			return serr
		}
		// A key has a third component, and it was reaching a path sink
		// unchecked. SplitOrgKey strips everything after the last '@' before it
		// splits owner/repo from the tool name, so the version inside an
		// org-scoped key -- "owner/repo:tool@1.2.3" -- is discarded by the
		// splitter and never seen by either of the checks above. It is then
		// promoted to the effective version at install time and composed into a
		// path, which is the whole reason versions are validated at all.
		//
		// "owner/repo@1.0:evil" and "owner/repo@x$(id):jq" both passed this
		// function before this check existed. The traversal case did not, but
		// only incidentally: SplitOrgKey rejects ".." anywhere in the key as a
		// substring, which catches that one shape and nothing else.
		//
		// This is the same rule the value half already gets, applied to the
		// component that happened to be spelled inside the key instead.
		if atIdx := strings.LastIndex(key, "@"); atIdx > 0 {
			if verr := pinsafe.ValidateRequested(key[atIdx+1:]); verr != nil {
				return fmt.Errorf("version in key %q: %w", key, verr)
			}
		}
	}

	return recipe.ValidateStrictName(bare)
}

// validateOrgSource checks an owner/repo coordinate.
//
// Deliberately wider than the bare-name rule: GitHub permits uppercase in both
// halves, so this must not be harmonized with ValidateStrictName however much
// the one-definition principle invites it. That principle governs the *name*
// rule; an owner is a different namespace with different legal characters, and
// collapsing the two would reject real repositories.
func validateOrgSource(source string) error {
	owner, repo, found := strings.Cut(source, "/")
	if !found {
		return fmt.Errorf("org-scoped source %q must be owner/repo", source)
	}
	for label, seg := range map[string]string{"owner": owner, "repository": repo} {
		if seg == "" {
			return fmt.Errorf("org-scoped source %q has an empty %s", source, label)
		}
		if seg == "." || seg == ".." {
			return fmt.Errorf("org-scoped source %q has an invalid %s %q", source, label, seg)
		}
		if strings.ContainsAny(seg, "\x00/\\") {
			return fmt.Errorf("org-scoped source %q has an invalid %s %q", source, label, seg)
		}
		if !orgSegmentPattern.MatchString(seg) {
			return fmt.Errorf("org-scoped source %q has an invalid %s %q "+
				"(letters, digits, '.', '_', '-')", source, label, seg)
		}
	}
	return nil
}
