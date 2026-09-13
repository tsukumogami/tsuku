package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// sourceState is where one project-named source stands.
type sourceState int

const (
	// sourceRegistered: the user already has it. Nothing to ask.
	sourceRegistered sourceState = iota
	// sourceApproved: the user said yes on this run, and it has not been
	// written yet. It is written once the install proceeds and not before.
	sourceApproved
	// sourceNeedsApproval: nobody approved it. Its tools are skipped.
	sourceNeedsApproval
	// sourceFailed: it could not be classified -- an invalid name, or
	// strict_registries. Its tools fail rather than being skipped, which is a
	// different thing and reported differently.
	sourceFailed
)

// projectSource is one unique source a .tsuku.toml named.
type projectSource struct {
	Name  string
	Tools []string
	State sourceState
	Err   error

	// NoTerminal records why approval was missing, so the message can say
	// "there was no terminal to ask on" rather than "you declined" to somebody
	// who was never asked.
	NoTerminal bool

	// ApprovedVia records which route approved it, for the provenance record.
	ApprovedVia string

	userCfg *userconfig.Config
}

// projectSourcePlan holds the decision about every source in one project
// install, so the decision can be made before the tool list is printed and the
// write can happen after the user proceeds.
//
// Nothing here writes until commit runs. That ordering is the feature: the
// install used to register during a pre-scan, before the "Proceed?" gate, so
// declining could not undo what agreeing had not yet been asked for.
type projectSourcePlan struct {
	// DeclaredIn is the resolved path of the .tsuku.toml that named these
	// sources. One project install reads one file, so one path covers all of
	// them.
	DeclaredIn string

	sources map[string]*projectSource
	order   []string
}

func newProjectSourcePlan(declaredIn string) *projectSourcePlan {
	// Resolved and absolute, because the record outlives the run: a relative
	// path or one through a symlink says nothing to somebody reading
	// `tsuku registry list` from a different directory a month later.
	if abs, err := filepath.Abs(declaredIn); err == nil {
		declaredIn = abs
	}
	if resolved, err := filepath.EvalSymlinks(declaredIn); err == nil {
		declaredIn = resolved
	}
	return &projectSourcePlan{
		DeclaredIn: declaredIn,
		sources:    map[string]*projectSource{},
	}
}

// add records that a tool is declared from a source.
func (p *projectSourcePlan) add(source, tool string) {
	s, ok := p.sources[source]
	if !ok {
		s = &projectSource{Name: source}
		p.sources[source] = s
		p.order = append(p.order, source)
		sort.Strings(p.order)
	}
	s.Tools = append(s.Tools, tool)
}

func (p *projectSourcePlan) empty() bool { return len(p.sources) == 0 }

// each iterates the sources in a stable order, so output does not depend on map
// iteration.
func (p *projectSourcePlan) each(fn func(*projectSource)) {
	for _, name := range p.order {
		fn(p.sources[name])
	}
}

func (p *projectSourcePlan) state(source string) (sourceState, bool) {
	s, ok := p.sources[source]
	if !ok {
		return sourceRegistered, false
	}
	return s.State, true
}

// classify settles what is already registered and what cannot be used at all.
// It writes nothing and makes no network call.
func (p *projectSourcePlan) classify() {
	p.each(func(s *projectSource) {
		class, err := classifySource(s.Name)
		if err != nil {
			s.State = sourceFailed
			s.Err = err
			printWarning(fmt.Sprintf("Warning: failed to register source %q: %v", s.Name, err))
			return
		}
		s.userCfg = class.UserCfg
		if class.Registered {
			s.State = sourceRegistered
			return
		}
		s.State = sourceNeedsApproval
	})
}

// consentInputs is what the consent decision is allowed to read.
//
// It is an explicit struct rather than a function reading globals so that the
// answer to "what can grant consent here?" is the field list. Notably absent:
// the environment. A detected CI environment grants nothing, and neither does
// anything in the project file.
type consentInputs struct {
	AutoApprove bool
	Interactive func() bool
	Ask         func(prompt string) bool
}

// decideConsent asks about each unregistered source.
//
// It runs after the tool list is printed and before the "Proceed?" gate, so the
// reader has seen what the project declares before being asked whether to trust
// where it comes from.
func (p *projectSourcePlan) decideConsent(in consentInputs) {
	p.each(func(s *projectSource) {
		if s.State != sourceNeedsApproval {
			return
		}
		if in.AutoApprove {
			s.State = sourceApproved
			s.ApprovedVia = userconfig.ApprovedViaYesFlag
			return
		}
		if !in.Interactive() {
			s.NoTerminal = true
			return
		}
		prompt := fmt.Sprintf("Register source %q, declared in %q, to install %s?",
			s.Name, p.DeclaredIn, strings.Join(s.Tools, ", "))
		if in.Ask(prompt) {
			s.State = sourceApproved
			s.ApprovedVia = userconfig.ApprovedViaPrompt
		}
	})
}

// commit writes every approved source in one save.
//
// One save rather than one per source: a partial write is the outcome nobody
// asked for, and the entries were all approved in the same breath.
func (p *projectSourcePlan) commit() error {
	var approved []*projectSource
	p.each(func(s *projectSource) {
		if s.State == sourceApproved {
			approved = append(approved, s)
		}
	})
	if len(approved) == 0 {
		return nil
	}

	userCfg := approved[0].userCfg
	if userCfg == nil {
		loaded, err := userconfig.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		userCfg = loaded
	}

	// Grouped by approval route so each group carries its own record, while
	// still going through the one writer. In practice a run uses one route for
	// everything, so this is almost always a single group.
	byRoute := map[string][]string{}
	for _, s := range approved {
		byRoute[s.ApprovedVia] = append(byRoute[s.ApprovedVia], s.Name)
	}
	routes := make([]string, 0, len(byRoute))
	for route := range byRoute {
		routes = append(routes, route)
	}
	sort.Strings(routes)

	// Through the one writer, not a second save of its own. Two writers would
	// make "the install path writes config.toml in exactly one place" a claim
	// about one door in a room with two, and that claim is what makes gating
	// consent here sufficient rather than merely necessary.
	for _, route := range routes {
		prov := sourceProvenance{ApprovedVia: route, DeclaredIn: p.DeclaredIn}
		if err := autoRegisterSource(userCfg, prov, byRoute[route]...); err != nil {
			return err
		}
	}
	for _, s := range approved {
		fmt.Fprintf(os.Stderr, "Auto-registered source %q\n", s.Name)
	}
	return nil
}

// activate builds the session providers for sources that may be used.
func (p *projectSourcePlan) activate(sysCfg *config.Config) {
	p.each(func(s *projectSource) {
		if s.State != sourceRegistered && s.State != sourceApproved {
			return
		}
		if err := addDistributedProvider(s.Name, sysCfg); err != nil {
			s.State = sourceFailed
			s.Err = err
			printWarning(fmt.Sprintf("Warning: failed to register source %q: %v", s.Name, err))
		}
	})
}

// reportSkipped names every source left unapproved, its declaring file, the
// tools that were skipped, and both ways to approve it.
//
// The declaring path and the tool names are quoted for the same reason the
// refusal line quotes its path: they come from a repository the invoking user
// may not control.
func (p *projectSourcePlan) reportSkipped() {
	p.each(func(s *projectSource) {
		if s.State != sourceNeedsApproval {
			return
		}
		why := "was not approved"
		if s.NoTerminal {
			why = "needs approval and there was no terminal to ask on"
		}
		fmt.Fprintf(os.Stderr,
			"\nSource %q %s, so %s %s skipped.\n"+
				"  Declared in: %q\n"+
				"  To approve it, either:\n"+
				"    tsuku install --yes\n"+
				"    tsuku registry add %s\n",
			s.Name, why, quotedList(s.Tools), pluralVerb(len(s.Tools)), p.DeclaredIn, s.Name)
	})
}

// anyNeedsApproval reports whether the run must exit with the needs-approval
// code.
func (p *projectSourcePlan) anyNeedsApproval() bool {
	found := false
	p.each(func(s *projectSource) {
		if s.State == sourceNeedsApproval {
			found = true
		}
	})
	return found
}

func quotedList(items []string) string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, fmt.Sprintf("%q", item))
	}
	return strings.Join(out, ", ")
}

// allToolsBlocked reports whether every declared tool belongs to a source that
// cannot be used.
//
// When that holds there is nothing left to proceed with, so asking "Proceed?"
// would be asking about an empty list. The run reports what it skipped and
// exits instead.
func allToolsBlocked(tools []toolEntry, plan *projectSourcePlan) bool {
	if len(tools) == 0 {
		return false
	}
	for _, t := range tools {
		if t.Distributed == nil {
			return false
		}
		st, ok := plan.state(t.Distributed.Source)
		if !ok || st == sourceRegistered || st == sourceApproved {
			return false
		}
	}
	return true
}
