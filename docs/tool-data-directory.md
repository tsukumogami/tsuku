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

There is no `tsuku` command that does this for you. Reinstalling nvm later will find the
directory still there and pick up where you left off.

## If you installed nvm before this existed

Earlier versions of tsuku pointed `NVM_DIR` at a directory that gets recycled, so your
Node versions may still be sitting in one. They move to `$TSUKU_HOME/data/nvm`
automatically the next time nvm updates — in the same step that repoints `NVM_DIR`, so
there is no moment where your shell is looking in the wrong place.

Until then nothing is broken: your shell and your data still agree, and `nvm ls` works.
`tsuku doctor` will mention it, and say which of the two situations you are in:

- **`WARN (using a legacy location)`** — working. Your Node versions are in an old
  location and your shell knows it. They will move on the next nvm update. Nothing to do.
- **`FAIL (data left behind in an old location)`** — your shell is pointing at the new
  directory but the data did not make it there, so `nvm ls` will come up empty. Run
  `tsuku doctor --fix` to retry the move.

If `--fix` cannot finish, it says so rather than reporting success. The two cases it
cannot repair are a file that already exists at the destination, and a move across a
filesystem boundary. Both print the paths involved; move them with `mv` and re-run
`tsuku doctor`.

The move never overwrites and never deletes. If anything goes wrong the worst outcome is
the same files in two places, which is why it is safe to let it run unattended.

## An existing nvm you installed some other way

tsuku does not read, write, or remove anything under `~/.nvm`. If you already had nvm
installed the usual way, tsuku's copy keeps its own Node versions in
`$TSUKU_HOME/data/nvm` and yours stay where they are.

## A note on `rm -rf $TSUKU_HOME`

This used to be a safe way to start over. It is not any more: `$TSUKU_HOME/data/` holds
things you cannot get back without re-downloading and re-installing them. Remove
`$TSUKU_HOME/tools`, `$TSUKU_HOME/bin`, and `$TSUKU_HOME/state.json` instead if you want
a clean slate while keeping what your tools are holding for you.
