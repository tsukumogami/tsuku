#!/usr/bin/env python3
"""Every scheduled workflow declares what happens when it fails, and it is wired up.

A declaration nothing is wired to is worse than no declaration: it reads as coverage and
delivers nothing. So this does not merely check that the comment blocks are present and
well-formed -- it compares the set of workflows declaring `escalation-policy: issue`
against the escalator's registry in BOTH directions and fails on any difference.

Usage:
    workflow-policy.py [--registry PATH] [--workflows DIR] [--ref GIT-REF]

Exit codes:
    0  every scheduled workflow declares a policy, and declarations and registry agree
    1  a declaration is missing, malformed, or disagrees with the registry
    2  operational error (registry missing or unparseable, no workflows found)

WHY THE TWO SOURCES ARE SEPARATE FILES

`.github/escalation-registry.yml` and the per-workflow comment blocks are independent by
construction. A single file holding both would be compared against itself and could never
fail. They agree today because they were written to agree; what the comparison buys is the
NEXT change, where someone adds a scheduled workflow and updates one of the two.

WHY --ref EXISTS

The escalator must read declarations from the default branch, never from a pull request's
head. Otherwise a pull request could register itself with the escalator, or redirect
another workflow's assignee, by editing a comment in its own diff -- the declaration would
be trusted before anyone reviewed it.

`--ref` reads every file from a git ref instead of the working tree -- the workflows **and
the registry**, because reading one from the ref and the other from disk would let a pull
request register itself by editing whichever half was still read locally. The escalator is
required to pass it. The CI check deliberately does NOT: a check exists to validate the
changes in front of it, and reading from the default branch would make it impossible to
add a workflow or correct a declaration. The two callers want opposite things from the
same parser, which is why this is an argument rather than a hardcoded behaviour.

WHY THIS IS PYTHON

The design names this `workflow-policy.sh`. It is Python for the same reason
`label-references.py` is: `on:` is parsed by YAML 1.1 as the boolean `True`, so a naive
reader silently sees no triggers at all and every workflow looks unscheduled -- which
would make this check pass having examined nothing. That is a failure mode worth spending
a language choice on.
"""

import argparse
import re
import subprocess
import sys
from pathlib import Path

import yaml

EXIT_PASS, EXIT_FAIL, EXIT_ERROR = 0, 1, 2

# The number of workflows independently known to carry a schedule trigger. Pinned, not
# derived: a check that counts what it finds and compares it to what it found cannot
# notice that it found nothing. Changing this number is a deliberate act that says a
# scheduled workflow was added or removed.
EXPECTED_SCHEDULED = 21

KEY = re.compile(r'^#\s*(?P<key>[a-z-]+):\s*(?P<value>.*?)\s*$')
DECLARATION_KEYS = {
    "escalation-policy",
    "escalation-assignee",
    "escalation-only-on",
    "escalation-reason",
    "escalation-none-kind",
    "escalation-tracking-issue",
    "coverage",
    "coverage-reason",
}
VALID_POLICY = {"issue", "none"}
VALID_NONE_KIND = {"deferred", "permanent"}
ISSUE_REF = re.compile(r"^#?(?P<number>[0-9]+)$")


def _repo_relative(path: Path) -> str:
    """A path git can resolve in `ref:path` form.

    `git show ref:/absolute/path` does not resolve, so an absolute path handed to --ref
    would fail as a missing file rather than as the caller error it is. Rebasing it on the
    repository root turns that into the lookup the caller meant.
    """
    if not path.is_absolute():
        return path.as_posix()
    root = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        capture_output=True, text=True, check=True,
    ).stdout.strip()
    return path.resolve().relative_to(root).as_posix()


def read_tree(workflows: Path, ref: str | None):
    """Yield (name, text) for every workflow file, from `ref` or from the working tree."""
    if ref is None:
        for p in sorted(list(workflows.glob("*.yml")) + list(workflows.glob("*.yaml")),
                        key=lambda p: p.name):
            yield p.name, p.read_text()
        return

    listing = subprocess.run(
        ["git", "ls-tree", "--name-only", f"{ref}:{_repo_relative(workflows)}"],
        capture_output=True, text=True, check=True,
    ).stdout.split()
    for name in sorted(n for n in listing if n.endswith((".yml", ".yaml"))):
        blob = subprocess.run(
            ["git", "show", f"{ref}:{_repo_relative(workflows)}/{name}"],
            capture_output=True, text=True, check=True,
        ).stdout
        yield name, blob


def declaration_of(text):
    """The declaration keys in a file's leading comment block.

    Only the run of comment lines before the first non-comment line is read. A key
    mentioned in prose further down the file is not a declaration, and treating it as one
    would let any workflow be registered by talking about it.
    """
    found = {}
    for raw in text.splitlines():
        if not raw.startswith("#"):
            if raw.strip() == "":
                continue
            break
        m = KEY.match(raw)
        if m and m.group("key") in DECLARATION_KEYS:
            found.setdefault(m.group("key"), m.group("value"))
    return found


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--registry", default=".github/escalation-registry.yml")
    ap.add_argument("--workflows", default=".github/workflows")
    ap.add_argument("--ref", default=None,
                    help="read from this git ref instead of the working tree; "
                         "the escalator passes the default branch")
    ap.add_argument("--expect-scheduled", type=int, default=EXPECTED_SCHEDULED)
    args = ap.parse_args()

    workflows = Path(args.workflows)
    registry_path = Path(args.registry)

    # The registry is read from the same place as the declarations. Reading declarations
    # from a ref while taking the registry off disk would defeat the point of --ref
    # entirely: a pull request could register itself with the escalator by editing the
    # registry alone, which is the exact substitution --ref exists to prevent.
    if args.ref is None:
        if not registry_path.is_file():
            print(f"::error::registry not found: {registry_path}", file=sys.stderr)
            return EXIT_ERROR
        registry_text = registry_path.read_text()
    else:
        try:
            registry_text = subprocess.run(
                ["git", "show", f"{args.ref}:{_repo_relative(registry_path)}"],
                capture_output=True, text=True, check=True,
            ).stdout
        except subprocess.CalledProcessError:
            print(f"::error::registry not found at {args.ref}:{registry_path}",
                  file=sys.stderr)
            return EXIT_ERROR
    try:
        registry_doc = yaml.safe_load(registry_text) or {}
    except yaml.YAMLError as e:
        print(f"::error file={registry_path}::registry is not valid YAML: {e}",
              file=sys.stderr)
        return EXIT_ERROR
    registered = registry_doc.get("registered")
    if not isinstance(registered, list) or not registered:
        print(f"::error file={registry_path}::registry declares no workflows; "
              "this check would have compared nothing", file=sys.stderr)
        return EXIT_ERROR
    registered = set(registered)

    scheduled = {}   # workflow name -> (filename, declaration, dual_trigger)
    failures = []
    deferred = []    # (filename, workflow name, tracking issue number)

    for filename, text in read_tree(workflows, args.ref):
        try:
            doc = yaml.safe_load(text)
        except yaml.YAMLError:
            continue
        if not isinstance(doc, dict):
            continue
        # `on:` is the YAML 1.1 boolean True. Reading doc["on"] alone finds nothing.
        on = doc.get("on", doc.get(True))
        if not (isinstance(on, dict) and "schedule" in on):
            continue
        wf_name = doc.get("name")
        if not wf_name:
            failures.append((filename, "scheduled workflow declares no `name:`, so it "
                                       "cannot be registered with the escalator"))
            continue
        scheduled[wf_name] = (filename, declaration_of(text), "pull_request" in on)

    # Zero-floor, and the pinned count. Both, because they catch different things: zero
    # means the scan broke, and a mismatch means the set changed without anyone saying so.
    if not scheduled:
        print(f"::error::found no scheduled workflows under {workflows}; "
              "the scan examined nothing", file=sys.stderr)
        return EXIT_ERROR
    if len(scheduled) != args.expect_scheduled:
        print(f"::error::found {len(scheduled)} scheduled workflow(s), expected "
              f"{args.expect_scheduled}. If a scheduled workflow was added or removed, "
              "update EXPECTED_SCHEDULED in this script in the same change.",
              file=sys.stderr)
        failures.append(("(count)", f"{len(scheduled)} != {args.expect_scheduled}"))

    declaring_issue = set()
    for wf_name, (filename, decl, dual) in sorted(scheduled.items()):
        policy = decl.get("escalation-policy")
        if policy is None:
            failures.append((filename, "no `escalation-policy:` declared"))
            continue
        if policy not in VALID_POLICY:
            failures.append((filename, f"`escalation-policy: {policy}` is not one of "
                                       f"{sorted(VALID_POLICY)}"))
            continue

        if policy == "issue":
            declaring_issue.add(wf_name)
            if not decl.get("escalation-assignee"):
                failures.append((filename, "`escalation-policy: issue` with no "
                                           "`escalation-assignee:`"))
        else:
            if not decl.get("escalation-reason"):
                failures.append((filename, "`escalation-policy: none` with no "
                                           "`escalation-reason:`; declining to escalate "
                                           "has to be said out loud"))

            # A `none` must say WHICH KIND it is. Neglect and intent otherwise produce an
            # identical file: someone writes a prose `none` meaning "for now", nobody
            # revisits it, and a month later it is indistinguishable from a deliberate
            # permanent exemption. Naming the kind costs the author nothing at the moment
            # they already know the answer.
            kind = decl.get("escalation-none-kind")
            if not kind:
                failures.append((filename, "`escalation-policy: none` with no "
                                           "`escalation-none-kind:`; say whether this is "
                                           "`deferred` (name the issue that owns the "
                                           "condition) or `permanent` (say why it will "
                                           "never escalate)"))
            elif kind not in VALID_NONE_KIND:
                failures.append((filename, f"`escalation-none-kind: {kind}` is not one of "
                                           f"{sorted(VALID_NONE_KIND)}"))
            elif kind == "deferred":
                ref = decl.get("escalation-tracking-issue", "")
                m = ISSUE_REF.match(ref.strip())
                if not m:
                    failures.append((filename, "`escalation-none-kind: deferred` needs "
                                               "`escalation-tracking-issue:` naming the "
                                               "issue number that owns the condition"))
                else:
                    deferred.append((filename, wf_name, int(m.group("number"))))

        coverage = decl.get("coverage")
        if coverage is None:
            failures.append((filename, "no `coverage:` declared"))
        elif coverage == "none" and not decl.get("coverage-reason"):
            failures.append((filename, "`coverage: none` with no `coverage-reason:`"))

        # A workflow that also runs on pull_request must scope escalation to the schedule,
        # or a contributor's failing branch files an issue against them.
        if dual and decl.get("escalation-only-on") != "schedule":
            failures.append((filename, "declares both `schedule` and `pull_request` but "
                                       "not `escalation-only-on: schedule`"))

    # A deferred `none` is valid only while the issue that owns its condition is open.
    #
    # This is what makes `none` a deferral rather than a disposal. An exemption that
    # outlives its reason is indistinguishable from an exemption nobody revisited, and
    # prose cannot tell those apart -- "until X lands" is only true while something reads
    # X. When the tracking issue closes, the condition that justified the exemption is
    # gone, so the declaration stops validating and says so loudly.
    #
    # It fails closed. If the issue state cannot be resolved -- no `gh`, no credentials,
    # no network -- that is an operational error, never a pass. A check that could not
    # look has not looked, and reporting success would be the defect this repository has
    # spent this milestone removing.
    for filename, wf_name, number in deferred:
        try:
            result = subprocess.run(
                ["gh", "issue", "view", str(number), "--json", "state", "-q", ".state"],
                capture_output=True, text=True, check=True, timeout=30,
            )
        except (subprocess.CalledProcessError, subprocess.TimeoutExpired, FileNotFoundError) as e:
            print(f"::error file={workflows.as_posix()}/{filename}::cannot resolve the "
                  f"state of tracking issue #{number}, so the deferred `none` in "
                  f"\"{wf_name}\" could not be validated: {e}", file=sys.stderr)
            return EXIT_ERROR
        state = result.stdout.strip().upper()
        if state != "OPEN":
            failures.append((filename, f"`escalation-none-kind: deferred` names tracking "
                                       f"issue #{number}, which is {state.lower()}. The "
                                       f"condition that justified not escalating has "
                                       f"resolved -- restore `escalation-policy: issue`, "
                                       f"or record a new reason."))

    # The two-way comparison. Each direction catches a different mistake, which is why
    # neither alone is enough: a workflow declaring `issue` that the escalator does not
    # watch is a promise nothing keeps, and a registered workflow that declares nothing is
    # the escalator acting on a policy no one wrote down.
    declared_not_registered = declaring_issue - registered
    registered_not_declared = registered - declaring_issue

    for wf_name in sorted(declared_not_registered):
        filename = scheduled[wf_name][0]
        print(f"::error file={workflows.as_posix()}/{filename}::\"{wf_name}\" declares "
              f"`escalation-policy: issue` but is not in {registry_path}, so nothing "
              "escalates it", file=sys.stderr)
    for wf_name in sorted(registered_not_declared):
        print(f"::error file={registry_path}::\"{wf_name}\" is registered with the "
              "escalator but no scheduled workflow declares "
              "`escalation-policy: issue` under that name", file=sys.stderr)

    for filename, reason in failures:
        print(f"::error::{filename}: {reason}", file=sys.stderr)

    dual_count = sum(1 for _, (_, _, dual) in scheduled.items() if dual)
    print(
        f"Workflow policy: {len(scheduled)} scheduled workflow(s), {len(declaring_issue)} "
        f"declaring `issue`, {dual_count} also on pull_request and scoped with "
        f"`escalation-only-on`; {len(registered)} registered with the escalator."
    )

    if failures or declared_not_registered or registered_not_declared:
        print(
            f"Workflow policy: {len(failures)} declaration problem(s), "
            f"{len(declared_not_registered)} declared-but-unregistered, "
            f"{len(registered_not_declared)} registered-but-undeclared.",
            file=sys.stderr,
        )
        return EXIT_FAIL
    return EXIT_PASS


if __name__ == "__main__":
    sys.exit(main())
