package autoinstall

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/project"
)

// refuse prints the refusal for e and returns it, so the caller can decline in
// one statement.
//
// The message is printed here rather than by cmd/tsuku, for the reason
// ErrSuggestOnly's instructions are: what the user is told when the run path
// declines to run something is a property of that path and not of whichever
// command drove it, and a second caller would otherwise have to reproduce it.
// cmd/tsuku's case for this error maps it to an exit code and prints nothing.
//
// It also puts the message where the configuration is. The version-specific
// path each declaration is reachable at is built from the same *config.Config
// the run path executes from, so a message assembled anywhere else would need
// that handed to it a second time.
func (r *Runner) refuse(e *AmbiguousDeclarationError) error {
	fmt.Fprintf(r.stderr, "%s", r.refusalMessage(e))
	return e
}

// refusalMessage is the whole refusal, terminated by a newline.
//
// R6 requires three things of it, and each declaration carries all three: the
// recipe name, the configuration key and version it was declared under, and a
// complete invocation that reaches that recipe rather than its sibling. The
// key is the load-bearing one -- two org-scoped keys naming one bare recipe
// produce two declarations whose Recipe is the same string, and a message
// built from recipe names alone names the same thing twice.
//
// Only the project's own declarations appear. The other providers the index
// knows about are not what made this ambiguous and naming them would send the
// reader to a file that does not mention them.
func (r *Runner) refusalMessage(e *AmbiguousDeclarationError) string {
	var b strings.Builder

	fmt.Fprintf(&b, "tsuku run: %d recipes in %s provide %q. Nothing was installed and nothing was run: "+
		"the file that was meant to settle which provider to use names more than one of them.\n\n",
		len(e.Declarations), e.configPath(), e.Command)

	b.WriteString("Each declaration, with the two commands that reach it:\n\n")
	for _, d := range e.Declarations {
		fmt.Fprintf(&b, "  %s %s, declared as %q\n", d.Recipe, declaredVersion(d), d.ConfigKey)
		fmt.Fprintf(&b, "    tsuku install %s\n", installArgument(d))
		fmt.Fprintf(&b, "    %s\n\n", r.reachIt(d, e.Command))
	}

	fmt.Fprintf(&b, "Removing all but one of those declarations makes `tsuku run %s` work.\n", e.Command)
	return b.String()
}

// configPath is the file the declarations came from, for the message.
//
// Every declaration in one set carries the same path -- they are built from
// one loaded configuration -- so the first one that has it answers for all.
func (e *AmbiguousDeclarationError) configPath() string {
	for _, d := range e.Declarations {
		if d.ConfigPath != "" {
			return d.ConfigPath
		}
	}
	return "the project configuration"
}

// declaredVersion is the version to show for a declaration. An omitted version
// means the same thing to the installer as `latest`, and saying so beats
// printing a blank where a version belongs.
func declaredVersion(d project.ProjectDeclaration) string {
	if d.Version == "" {
		return "latest"
	}
	return d.Version
}

// installArgument is what to pass `tsuku install` to install one declaration.
//
// The configuration key is the argument rather than the recipe name, because
// the key is what carries the source: `tsuku install koto` and `tsuku install
// org-a/registry:koto` install different recipes, and the second is the one an
// org-scoped declaration asked for. A key that already carries its own
// `@version` is passed through as written.
func installArgument(d project.ProjectDeclaration) string {
	if strings.Contains(d.ConfigKey, "@") {
		return d.ConfigKey
	}
	return d.ConfigKey + "@" + declaredVersion(d)
}

// reachIt is the second command of a declaration's invocation: what to run,
// once the install above it has finished, to get that recipe's copy of the
// command rather than its sibling's.
//
// For an exact pin that is the version-specific path, which goes on naming
// that recipe however many other providers of the command are installed
// afterwards.
//
// A `latest`, prefix or channel declaration has no such path to name: which
// version directory the install lands in is decided by a resolution that has
// not happened, and this refusal happens before any install. What is knowable
// is where the install links the command -- $TSUKU_HOME/tools/current, the
// directory tsuku tells users to put on PATH, and the same one Run execs an
// undeclared installed command from. The link is claimed by whichever recipe
// was installed last, which is the recipe the line above it just installed:
// the pair is a sequence to follow in order, not two independent facts.
func (r *Runner) reachIt(d project.ProjectDeclaration, command string) string {
	if install.PinLevelFromRequested(d.Version) != install.PinExact {
		return filepath.Join(r.cfg.CurrentDir, command)
	}
	return filepath.Join(r.cfg.ToolBinDir(d.Recipe, d.Version), command)
}
