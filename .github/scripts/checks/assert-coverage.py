#!/usr/bin/env python3
"""Assert run receipts against declared sets (#2608).

A scheduled workflow that declares `coverage: items` writes a receipt as it works: one
JSON line per item it handled, `{"item": ..., "outcome": ...}`, appended where the work
happens. This script compares each leg's receipt against a declared set that came from
somewhere other than the leg, and fails the run when the leg did not cover it.

Usage:
  assert-coverage.py --leg NAME=DECLARED_FILE:RECEIPT_FILE [--leg ...]
                     --pass-outcomes a,b,... --fail-outcomes c,d,...

The legs are named on the command line by the workflow, not discovered from the receipts
that happened to arrive. A leg killed at its timeout uploads nothing, and a check that
derived "expected" from what it found would never notice it was missing.

For each leg it prints one line -- `<leg>: declared N, attempted M, ...` -- and fails the
leg when any of these hold:

  - the declared file is missing (nothing said what the leg should cover)
  - the receipt is missing (the leg did not run, or was killed before uploading)
  - attempted is zero
  - a declared item has no receipt line (not validated)
  - a receipt line names an item that was not declared
  - an item has an outcome in --fail-outcomes
  - a line is malformed, names an outcome in neither list, or repeats an item

`attempted` counts declared items that got a line, whatever the outcome. An item skipped
by rule is examined, named and counted; it is not dropped. Both numbers are printed for
every leg, passing or not, so a healthy run shows what it covered.

Exit: 0 every leg covered, 1 any leg did not, 2 usage error.
"""

import argparse
import hashlib
import json
import os
import sys

MAX_RECEIPT_BYTES = 16 * 1024 * 1024
EXAMPLES = 10


def parse_leg(spec):
    name, sep, rest = spec.partition("=")
    declared, sep2, receipt = rest.partition(":")
    if not (sep and sep2 and name and declared and receipt):
        raise argparse.ArgumentTypeError(f"--leg wants NAME=DECLARED:RECEIPT, got {spec!r}")
    return name, declared, receipt


def outcome_set(text):
    return {o for o in (p.strip() for p in text.split(",")) if o}


def examples(items):
    items = sorted(items)
    shown = ", ".join(items[:EXAMPLES])
    more = len(items) - EXAMPLES
    return shown + (f", and {more} more" if more > 0 else "")


def check_leg(name, declared_path, receipt_path, pass_outcomes, fail_outcomes):
    """Return (ok, lines) for one leg."""
    out = []
    if not os.path.isfile(declared_path):
        out.append(f"{name}: no declared set ({declared_path} is missing); nothing said what this leg should cover")
        return False, out

    with open(declared_path, encoding="utf-8") as f:
        declared = {line.strip() for line in f if line.strip()}
    digest = hashlib.sha256("\n".join(sorted(declared)).encode()).hexdigest()[:12]

    if not os.path.isfile(receipt_path):
        out.append(f"{name}: declared {len(declared)}, attempted 0 -- no receipt ({receipt_path} is missing): "
                   f"the leg did not run, or was killed before uploading")
        return False, out
    if os.path.getsize(receipt_path) > MAX_RECEIPT_BYTES:
        out.append(f"{name}: receipt is larger than {MAX_RECEIPT_BYTES} bytes; refusing to parse it")
        return False, out

    problems = []
    outcomes = {}
    seen = set()
    duplicates = set()
    malformed = []
    unknown = set()
    with open(receipt_path, encoding="utf-8") as f:
        for n, raw in enumerate(f, 1):
            if not raw.strip():
                continue
            try:
                rec = json.loads(raw)
                item, outcome = rec["item"], rec["outcome"]
                if not isinstance(item, str) or not isinstance(outcome, str) or not item:
                    raise ValueError
            except (ValueError, KeyError, TypeError):
                malformed.append(n)
                continue
            if item in seen:
                duplicates.add(item)
                continue
            seen.add(item)
            if outcome not in pass_outcomes and outcome not in fail_outcomes:
                unknown.add(outcome)
            outcomes.setdefault(outcome, set()).add(item)

    attempted = seen & declared
    missing = declared - seen
    undeclared = seen - declared
    failed = set().union(*(outcomes.get(o, set()) for o in fail_outcomes)) if fail_outcomes else set()

    counts = ", ".join(f"{o} {len(outcomes[o])}" for o in sorted(outcomes))
    head = (f"{name}: declared {len(declared)}, attempted {len(attempted)}, "
            f"not validated {len(missing)}, undeclared {len(undeclared)}"
            f" (declared set {digest}{'; ' + counts if counts else ''})")

    if not attempted:
        problems.append("attempted nothing")
    if missing:
        problems.append(f"{len(missing)} declared item(s) have no receipt line: {examples(missing)}")
    if undeclared:
        problems.append(f"{len(undeclared)} receipt line(s) name undeclared items: {examples(undeclared)}")
    if failed:
        problems.append(f"{len(failed)} item(s) failed: {examples(failed)}")
    if malformed:
        problems.append(f"{len(malformed)} malformed receipt line(s), first at line {malformed[0]}")
    if unknown:
        problems.append(f"unknown outcome(s): {', '.join(sorted(unknown))}")
    if duplicates:
        problems.append(f"{len(duplicates)} item(s) appear more than once: {examples(duplicates)}")

    out.append(head)
    out.extend(f"  {p}" for p in problems)
    return not problems, out


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--leg", action="append", type=parse_leg, required=True)
    ap.add_argument("--pass-outcomes", type=outcome_set, required=True)
    ap.add_argument("--fail-outcomes", type=outcome_set, required=True)
    try:
        args = ap.parse_args()
    except SystemExit as e:
        sys.exit(2 if e.code else 0)

    overlap = args.pass_outcomes & args.fail_outcomes
    if overlap:
        print(f"::error::outcome(s) in both lists: {', '.join(sorted(overlap))}", file=sys.stderr)
        sys.exit(2)

    names = [leg[0] for leg in args.leg]
    if len(set(names)) != len(names):
        print("::error::a leg is named twice", file=sys.stderr)
        sys.exit(2)

    all_ok = True
    for name, declared, receipt in args.leg:
        ok, lines = check_leg(name, declared, receipt, args.pass_outcomes, args.fail_outcomes)
        for line in lines:
            print(line)
        if not ok:
            all_ok = False
            print(f"::error::{name} did not cover its declared set: {lines[0]}")

    print(f"Coverage: {len(args.leg)} leg(s) asserted, {'all covered' if all_ok else 'NOT all covered'}.")
    sys.exit(0 if all_ok else 1)


if __name__ == "__main__":
    main()
