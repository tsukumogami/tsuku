#!/usr/bin/env python3
"""Every label a workflow names must be declared in the label manifest.

Nothing otherwise connects a `--label` argument to the set of labels that exist, so a
workflow can name a label nobody ever created and fail only on the day it finally tries
to file an issue. That failure surfaces as the escalation step dying, which is the worst
place to learn about it.

Usage:
    label-references.py [--manifest PATH] [--workflows DIR]

Exit codes:
    0  every reference resolves to a declared label
    1  a reference names a label that is not declared, a reference cannot be reduced to
       a literal, or the scan found no references at all
    2  operational error (manifest missing or unparseable)

This check is Python rather than shell, unlike its siblings in this directory, because
resolving a label through a shell variable needs a second pass over the file and the
array form needs real tokenising. Both are possible in awk and neither is legible there.

WHAT COUNTS AS A REFERENCE

  1. `--label X` and `--add-label X` on a shell command line. X may be double-quoted,
     single-quoted, or bare.
  2. `labels: X` inside an actions/github-script body. X may be a quoted string or a
     bracketed array.

A comma-joined value names several labels: `--label "a,b"` is two references, because
that is how `gh` reads it.

WHAT IS EXCLUDED, AND WHY IT IS A RULE

A `labels:` value that is wholly a single `${{ }}` expression is a workflow input, not an
issue-label list -- `labels: ${{ steps.meta.outputs.labels }}` feeding a container build
is a set of OCI image labels and has nothing to do with issues. The exclusion is stated
as a property of the value, never as a file-and-line allowlist: an allowlist stops
covering that file the moment the step moves, and would have to be re-derived by whoever
next touched it.

A value that merely *contains* an expression or a variable among other text is NOT
excluded. It cannot be reduced to a literal, so it is reported as unresolvable rather
than skipped. Silently skipping what the check cannot parse is the defect this check
exists to prevent, one level up.

RESOLVING A VARIABLE

`$VAR` or `${VAR}` resolves when the file contains exactly one assignment of the form
`VAR=<literal>`. Zero assignments means the reference is unresolvable. More than one
means the value depends on which branch ran, which this check will not guess at. Both
are reported with file and line.
"""

import argparse
import re
import sys
from pathlib import Path

EXIT_PASS, EXIT_FAIL, EXIT_ERROR = 0, 1, 2

# `- name: "x"` / `  color: "y"` / `  description: "z"` -- the shape label-manifest.sh
# writes and reads. Deliberately narrow: a malformed entry fails here rather than
# silently contributing no labels and letting the comparison pass against a short set.
MANIFEST_NAME = re.compile(r'^- name: "(?P<name>[^"]*)"\s*$')

# --label / --add-label followed by "quoted", 'quoted', or a bare token.
FLAG_REF = re.compile(
    r'--(?:add-)?label[=\s]+'
    r'(?:"(?P<dq>[^"]*)"|\'(?P<sq>[^\']*)\'|(?P<bare>[^\s"\';|&)\\]+))'
)

# `labels:` followed by a quoted string or a bracketed array, in a github-script body.
SCRIPT_REF = re.compile(
    r'(?<![-\w])labels:\s*'
    r'(?P<value>\[[^\]]*\]|"[^"]*"|\'[^\']*\'|\$\{\{.*?\}\}|[^\s,}]+)'
)

# A value that is wholly one ${{ }} expression, with nothing else in it.
WHOLLY_EXPRESSION = re.compile(r'^\$\{\{[^}]*\}\}$')

# VAR="literal" / VAR='literal' / VAR=literal, at the start of a shell line.
ASSIGNMENT = re.compile(
    r'^\s*(?P<var>[A-Za-z_][A-Za-z0-9_]*)='
    r'(?:"(?P<dq>[^"$]*)"|\'(?P<sq>[^\']*)\'|(?P<bare>[^\s"\'$]+))\s*$'
)

VARIABLE = re.compile(r'^\$\{?(?P<var>[A-Za-z_][A-Za-z0-9_]*)\}?$')


class Reference:
    """One label name, and the site that named it."""

    def __init__(self, name, path, line, raw, resolved_from=None):
        self.name = name
        self.path = path
        self.line = line
        self.raw = raw
        self.resolved_from = resolved_from

    def where(self):
        return f"{self.path}:{self.line}"

    def how(self):
        if self.resolved_from:
            return f" (via ${self.resolved_from})"
        return ""


class Unresolvable:
    def __init__(self, path, line, raw, reason):
        self.path = path
        self.line = line
        self.raw = raw
        self.reason = reason

    def where(self):
        return f"{self.path}:{self.line}"


def load_manifest(path):
    if not path.is_file():
        print(f"::error::manifest not found: {path}", file=sys.stderr)
        sys.exit(EXIT_ERROR)
    names = set()
    for raw in path.read_text().splitlines():
        m = MANIFEST_NAME.match(raw)
        if m:
            names.add(m.group("name"))
    if not names:
        print(
            f"::error file={path}::manifest declares no labels; "
            "this check would have examined nothing",
            file=sys.stderr,
        )
        sys.exit(EXIT_ERROR)
    return names


def assignments_in(text):
    """Every VAR=literal in a file, as var -> list of values.

    A list rather than a single value on purpose: the count is what decides whether a
    reference through that variable is resolvable, and a silent last-wins would let an
    ambiguous reference pass.
    """
    found = {}
    for raw in text.splitlines():
        m = ASSIGNMENT.match(raw)
        if not m:
            continue
        value = m.group("dq")
        if value is None:
            value = m.group("sq")
        if value is None:
            value = m.group("bare")
        found.setdefault(m.group("var"), []).append(value)
    return found


def split_names(value):
    return [part.strip() for part in value.split(",") if part.strip()]


def array_items(value):
    """Items of a `['a', "b"]` literal."""
    return re.findall(r'"([^"]*)"|\'([^\']*)\'', value[1:-1])


def scan_file(path, refs, unresolvable):
    text = path.read_text()
    assigns = assignments_in(text)
    rel = path.as_posix()

    def emit(value, lineno, raw):
        """Turn one extracted value into references, or into an unresolvable finding."""
        var = VARIABLE.match(value)
        if var:
            name = var.group("var")
            values = assigns.get(name, [])
            if len(values) == 1:
                for item in split_names(values[0]):
                    refs.append(Reference(item, rel, lineno, raw, resolved_from=name))
            elif not values:
                unresolvable.append(
                    Unresolvable(rel, lineno, raw, f"${name} has no assignment in this file")
                )
            else:
                unresolvable.append(
                    Unresolvable(
                        rel, lineno, raw,
                        f"${name} is assigned {len(values)} times; which one reaches this "
                        "line depends on control flow",
                    )
                )
            return

        if "$" in value:
            unresolvable.append(
                Unresolvable(rel, lineno, raw, "value mixes an expansion with other text")
            )
            return

        for item in split_names(value):
            refs.append(Reference(item, rel, lineno, raw))

    for lineno, raw in enumerate(text.splitlines(), start=1):
        stripped = raw.strip()
        if stripped.startswith("#"):
            continue

        for m in FLAG_REF.finditer(raw):
            value = m.group("dq")
            if value is None:
                value = m.group("sq")
            if value is None:
                value = m.group("bare")
            emit(value, lineno, stripped)

        m = SCRIPT_REF.search(raw)
        if m:
            value = m.group("value").strip()
            if WHOLLY_EXPRESSION.match(value):
                continue  # a workflow input, not an issue label -- see module docstring
            if value.startswith("["):
                for dq, sq in array_items(value):
                    emit(dq or sq, lineno, stripped)
            elif value[0] in "\"'":
                emit(value[1:-1], lineno, stripped)
            else:
                emit(value, lineno, stripped)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--manifest", default=".github/labels.yml")
    ap.add_argument("--workflows", default=".github/workflows")
    args = ap.parse_args()

    manifest = Path(args.manifest)
    workflows = Path(args.workflows)
    if not workflows.is_dir():
        print(f"::error::workflow directory not found: {workflows}", file=sys.stderr)
        return EXIT_ERROR

    declared = load_manifest(manifest)

    files = sorted(
        list(workflows.glob("*.yml")) + list(workflows.glob("*.yaml")),
        key=lambda p: p.name,
    )
    if not files:
        print(f"::error::no workflow files under {workflows}", file=sys.stderr)
        return EXIT_ERROR

    refs, unresolvable = [], []
    for path in files:
        scan_file(path, refs, unresolvable)

    # Zero-reference floor. A check that scanned every workflow and found nothing to
    # check has not passed; it has failed to read them.
    if not refs and not unresolvable:
        print(
            f"::error::scanned {len(files)} workflow file(s) and found no label "
            "reference at all; the extractor is not matching anything",
            file=sys.stderr,
        )
        return EXIT_FAIL

    missing = [r for r in refs if r.name not in declared]
    sites = {(r.path, r.line) for r in refs}

    for u in sorted(unresolvable, key=lambda u: (u.path, u.line)):
        print(
            f"::error file={u.path},line={u.line}::label reference cannot be reduced to "
            f"a literal: {u.reason}",
            file=sys.stderr,
        )
        print(f"    {u.where()}: {u.raw}", file=sys.stderr)

    if missing:
        by_name = {}
        for r in missing:
            by_name.setdefault(r.name, []).append(r)
        for name in sorted(by_name):
            where = ", ".join(f"{r.where()}{r.how()}" for r in sorted(by_name[name], key=lambda r: (r.path, r.line)))
            print(
                f"::error::label \"{name}\" is referenced but not declared in "
                f"{manifest}: {where}",
                file=sys.stderr,
            )

    resolved_names = {r.name for r in refs}
    print(
        f"Label references: {len(refs)} reference(s) at {len(sites)} site(s) across "
        f"{len({r.path for r in refs})} workflow(s), naming {len(resolved_names)} "
        f"distinct label(s); {len(declared)} declared in the manifest."
    )

    if missing or unresolvable:
        summary = []
        if missing:
            summary.append(
                f"{len({r.name for r in missing})} undeclared name(s) at "
                f"{len({(r.path, r.line) for r in missing})} site(s)"
            )
        if unresolvable:
            summary.append(f"{len(unresolvable)} unresolvable reference(s)")
        print(f"Label references: {'; '.join(summary)}.", file=sys.stderr)
        return EXIT_FAIL

    return EXIT_PASS


if __name__ == "__main__":
    sys.exit(main())
