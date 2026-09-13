---
schema: brief/v1
status: Draft
problem: |
  A `.tsuku.toml` now decides what `tsuku run` installs unprompted and which
  sources `tsuku install` registers, yet tsuku applies whatever file it finds as
  if the user wrote it. A file planted above a checkout, or shipped in a cloned
  repository, can choose where a user's tools come from without asking them.
outcome: |
  A developer can trust that a project file changes their tools only in ways
  they'd agree to. Default-registry tools behave as today, anything tsuku
  declines or needs approval for is reported on stderr, and nothing that
  outlives the command happens without someone saying yes.
---

# BRIEF: Project Config Trust

## Status

Draft

## Problem Statement

Before v0.14.0 a `.tsuku.toml` could only activate tools the user had already
installed. Since v0.14.0 it does more. It decides whether `tsuku run` installs a
declared tool without prompting, and it names recipe sources that `tsuku install`
adds to the user's global configuration. The file comes from whichever directory
tsuku finds it in, and tsuku treats it as if the person running the command had
written it. Nothing checks that assumption, and a user has no point at which they
decide whether a file they didn't write gets to choose where their tools come
from.

That gap shows up three ways, each reproduced against v0.14.0:

- **A file the user never saw applies to them.** Discovery walks up from the
  working directory and stops at `$HOME`. A checkout outside `$HOME`, such as
  `/tmp/shared-work` or `/srv/app`, never passes through `$HOME`, so the walk
  continues to `/`. Any local user can create `/tmp/.tsuku.toml`, and it then
  applies to every other user working beneath `/tmp`, with nothing on screen to
  say where the configuration came from (tsukumogami/tsuku#2555).
- **A file can register a permanent recipe source.** When a repository's config
  names an org-scoped tool such as `"owner/repo:tool"`, `tsuku install` adds
  `owner/repo` to `config.toml`. With a terminal attached it asks first. Without
  one (CI, a piped script, an agent) it registers the source silently, and
  `tsuku install --dry-run` does the same. The entry outlives the directory that
  caused it. Every later recipe resolution, for every project, consults it, and
  nothing records which file asked for it (tsukumogami/tsuku#2552).
- **A file can get a tool from a non-default source installed with no prompt.**
  `tsuku run` drops its install prompt for any command the project declares,
  including under a key that names a source other than the default registry.
  Today that path installs whatever the binary index holds rather than the named
  source, but once the previous problem has registered a source and something has
  been installed from it, a declaration can pull from that source silently
  (tsukumogami/tsuku#2559).

The three share one cause: a file's presence is being read as the user's
consent. A config found above the checkout is taken as the user's own, and a
missing terminal is taken as a yes. Simply refusing such files has its own
failure mode. A config that tsuku quietly ignores looks, from the user's side,
like tools that stopped working for no reason, so refusing a file is only half a
fix unless the user is told.

## User Outcome

A developer can work in a repository they just cloned, a shared host's scratch
directory, or a container volume, and trust that a project file changes their
tools only in ways they would have agreed to. The repositories they already use
keep working. Tools declared from the default registry activate and install
exactly as before, with no new question, and a checkout under `/srv` or on a
mounted volume still finds its own `.tsuku.toml`.

When tsuku declines to honor a file, or needs a yes before using a source a file
named, it says so on stderr, naming the file and the source, instead of acting
silently or ignoring silently. A person at a terminal is asked, and the question
names the source rather than only the tool. A CI job or agent that has nobody to
ask stops and says what it would have needed, and the machine's set of recipe
sources is unchanged. A dry run changes nothing. When a source does get added
because a project asked for it, the user can later tell which file asked.

## User Journeys

### Journey 1: Developer on a shared host, with a config planted above them

A developer on a shared Linux build machine works in `/tmp/shared-work`, where
another account has left a `/tmp/.tsuku.toml`. They open a shell with tsuku's
activation hook, or run `tsuku shell`. tsuku doesn't apply the planted file, and
tells them which file it skipped and why, without mixing that into the shell code
the hook evaluates. Their `PATH` is what it would be with no file there at all.

### Journey 2: Developer evaluating an unfamiliar repository

A developer clones a repository to try it out. Its `.tsuku.toml` declares a tool
from `someorg/recipes`. They run `tsuku install --dry-run` to see what the project
wants. The output reports that the source would need approval, and
`config.toml` is untouched. When they then run `tsuku install` at their terminal,
they're asked about `someorg/recipes` by name, with the declaring file shown. If
they decline, nothing is registered and the tools from other sources still
install.

### Journey 3: CI pipeline with nobody to ask

A maintainer's pipeline runs `tsuku install` non-interactively in a repository
that declares an org-scoped source. The job fails, saying which source it would
need and which file named it, and `config.toml` is left as it was. The
maintainer approves the source once, explicitly, in a way that's visible in the
pipeline definition or the image. Later runs go through, and the registration
shows which config caused it.

### Journey 4: Developer running a declared command

A developer in a project that declares its tools types `tsuku run jq`, or hits a
missing command through the command-not-found hook. A tool from the default
registry installs and runs without a prompt, as it does today. A tool whose
declaration names a different source is not waved through. The developer is
asked, the prompt names that source, and without a terminal the run stops with
a message rather than installing.

### Journey 5: Container user with a checkout outside `$HOME`

A developer builds in a container where the checkout is a mounted volume at
`/srv/app`. They run `tsuku install` there, and the project's own `.tsuku.toml`
is found and applied, as it is today. This journey pulls against Journey 1: both
files sit outside `$HOME`, and a file the developer didn't create can be
legitimate here and hostile there. Whatever tells the two apart has to keep this
layout working. Where it can't decide in the developer's favour, it tells them
why and what they can do about it instead of failing without explanation.

## Scope Boundary

**In scope:**

- Which `.tsuku.toml` files tsuku reads, for every command that discovers one:
  shell activation and the prompt hook, `tsuku run` and the command-not-found
  shim, and `tsuku install` with no arguments.
- What a user is told when tsuku finds a config and declines to use it.
- How a recipe source that only a project config named gets approved, across
  interactive runs, non-interactive runs, and `--dry-run`.
- How `tsuku run`'s prompt for a declared tool relates to the source the
  declaration names.
- Traceability of source registrations a project caused.
- The design record's description of the discovery ceiling variable, which
  currently reads as a mitigation in force when it's opt-in and unset by default.

**Out of scope:**

- Validation of tool names and versions inside `.tsuku.toml`. That's
  tsukumogami/tsuku#2553, already fixed, and nothing here re-does it.
- New prompts for tools declared from the default registry. The change must not
  turn into "ask about everything", which would bury the prompts that matter.
- Changes to `strict_registries`, to explicit `tsuku registry add`, or to sources
  named directly on the command line. Those are the user's own choices and keep
  working as they do.
- Other side effects of `--dry-run` from steps that run before any command, such
  as update checks writing caches. That's tsukumogami/tsuku#2549; only the
  project config's source registration is covered here.
- Startup cost proportional to the number of configured registries
  (tsukumogami/tsuku#2548).
- Cleaning up sources that earlier versions already auto-registered on users'
  machines. Existing entries stay where they are.
- Two org-scoped declarations that reduce to the same tool name
  (tsukumogami/tsuku#2561), fish shell activation (tsukumogami/tsuku#2556), and
  the stale binary-index check (tsukumogami/tsuku#2530).

## References

- `docs/designs/current/DESIGN-org-scoped-project-config.md`: where
  config-driven source registration was introduced, including its note on the
  trust boundary it shifted.
- `docs/designs/current/DESIGN-shell-env-activation.md`: the activation design
  whose mitigation list carries the ceiling-variable claim.
- `docs/designs/current/DESIGN-autoinstall-mode-resolution.md`: the consent-mode
  model `tsuku run` uses, including the elevation for declared commands.
