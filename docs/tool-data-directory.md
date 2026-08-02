# Tool data directory

Most tools tsuku installs are just programs. A few also hold things for you, and those
things have to outlive any single version of the tool.

nvm is the clearest case. Everything you install through it — every Node version, every
global npm package under those versions, the `default` alias that decides what a new
shell gets — lives under `$NVM_DIR`. If that pointed at the directory tsuku installs nvm
into, upgrading nvm would leave all of it behind: a new version means a new directory,
and tsuku eventually reclaims the old one.

So it does not. Tool data lives in its own tree:

```
$TSUKU_HOME/
├── tools/          # the programs, one directory per tool version, reclaimed over time
├── data/           # your things, kept
│   └── nvm/
│       ├── versions/   # every Node version you installed
│       ├── alias/      # including default
│       └── .cache/
└── ...
```

`$TSUKU_HOME/data/` is the one tree in `$TSUKU_HOME` that tsuku never deletes from.
Upgrading a tool does not touch it, reinstalling does not touch it, and background
cleanup does not touch it.

## What removal does

`tsuku remove nvm` removes nvm. It leaves `$TSUKU_HOME/data/nvm` alone and prints the
path.

That is deliberate, and it is asymmetric: tsuku will put your data somewhere it can
promise to keep, and will not take it away. If you want the space back, delete the
directory yourself:

```bash
rm -rf "$TSUKU_HOME/data/nvm"
```

There is no `tsuku` command that does this for you, and nothing reports that the
directory is there or how large it has grown. That gap is tracked in issue #2477.
Reinstalling nvm later will find the directory still there and pick up where you left
off.

## If you installed nvm before this existed

Your Node versions are **not** moved for you. The next time nvm updates, `NVM_DIR`
starts naming `$TSUKU_HOME/data/nvm` and `nvm ls` comes up empty until you move them.
Where they are depends on which tsuku installed them, and the two cases differ in
urgency.

**The common case: `$TSUKU_HOME/share/shell.d/`.** Every released tsuku up to v0.12.1
never exported `NVM_DIR` at all, so nvm self-located to wherever its script was sourced
from and installed there.

```bash
mv "$TSUKU_HOME"/share/shell.d/{versions,alias,.cache} "$TSUKU_HOME/data/nvm/"
```

Nothing is lost while you get to this — nothing garbage-collects `share/shell.d`, so the
installs sit there indefinitely.

**The urgent case: `$TSUKU_HOME/tools/nvm-<version>/`.** If you installed or updated nvm
from an unreleased build after the shell.d lifecycle fix landed, `NVM_DIR` did resolve —
to the versioned tool directory. That directory *is* reclaimed, on a 7-day retention
timer, by the background updater. Check for it and move it first:

```bash
ls -d "$TSUKU_HOME"/tools/nvm-*/versions 2>/dev/null   # anything listed is at risk
mv "$TSUKU_HOME"/tools/nvm-<version>/{versions,alias,.cache} "$TSUKU_HOME/data/nvm/"
```

In both cases move the named directories rather than using `mv src/* dst/`, which skips
dotfiles and would silently leave `.cache` — your download cache — behind.

Automatic migration was deliberately left out. Doing it properly means recipes being
able to describe their own data layout and carry their own migration steps, so that
knowledge ships and versions with the recipe instead of being compiled into the CLI.
That is a design in its own right rather than a special case for one tool, tracked in
issue #2472.

## An existing nvm you installed some other way

tsuku does not read, write, or remove anything under `~/.nvm`. If you already had nvm
installed the usual way, tsuku's copy keeps its own Node versions in
`$TSUKU_HOME/data/nvm` and yours stay where they are.

## A note on `rm -rf $TSUKU_HOME`

This used to be a safe way to start over. It is not any more: `$TSUKU_HOME/data/` holds
things you cannot get back without re-downloading and re-installing them. Remove
`$TSUKU_HOME/tools`, `$TSUKU_HOME/bin`, and `$TSUKU_HOME/state.json` instead if you want
a clean slate while keeping what your tools are holding for you.

There is no `tsuku uninstall` that would do this for you and tell you what it is about to
destroy. That, and the absence of any way to see or reclaim tool data, is issue #2477.
