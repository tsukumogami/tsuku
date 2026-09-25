#!/usr/bin/env python3
"""Two assertions about workflows, stated separately because they catch different things.

1. NO ISSUE-FILING STEP SUPPRESSES ITS OWN FAILURE.

   A step that files or edits an issue is a delivery step. If it swallows its error, the
   run goes green having told nobody, which is the defect this repository spent a
   milestone learning it could not see. The check enumerates every such step, reports the
   count, and fails if that count is zero -- a check that found no delivery steps has not
   verified that none suppress, it has failed to find them.

2. NO WORKFLOW CONTAINS AN EMPTY GITHUB EXPRESSION.

   Asserted on the class, not on one construct. GitHub evaluates expressions ANYWHERE in
   a workflow file -- `run:` blocks and comments included -- and an empty one is a syntax
   error that rejects the workflow before job creation. The resulting run creates no jobs
   and is recorded under the file's PATH rather than its declared name, so it is invisible
   to anything matching on name, which is every listener this repository has.

   This is not hypothetical. Both escalation workflows failed this way on their first
   push, twice, and nothing reported it. The cause was three comments explaining that
   expressions must not be interpolated into a `run:` block, each writing an empty
   expression literally while saying so. A comment is only a comment to a reader that
   agrees it is one; the expression evaluator does not.

Usage:
    escalation-hygiene.py [--workflows DIR]

Exit codes:
    0  no suppressed delivery step, no empty expression
    1  a suppression, an empty expression, or a zero floor
    2  operational error
"""

import argparse
import re
import sys
from pathlib import Path

import yaml

EXIT_PASS, EXIT_FAIL, EXIT_ERROR = 0, 1, 2

# What counts as filing or editing an issue.
DELIVERS = re.compile(
    r"gh\s+issue\s+(create|comment|close|edit|reopen)"
    r"|issues\.(create|createComment|update)"
    r"|/issues\b.*-X\s*(POST|PATCH)",
    re.IGNORECASE,
)

# Suppressions that would hide a delivery failure.
SUPPRESSIONS = [
    (re.compile(r"2>\s*/dev/null"), "2>/dev/null"),
    (re.compile(r"\|\|\s*true\b"), "|| true"),
    (re.compile(r"\|\|\s*echo\b"), "|| echo (substitutes a value for a failure)"),
]

# An expression whose body is empty or only whitespace.
EMPTY_EXPRESSION = re.compile(r"\$\{\{\s*\}\}")


def delivery_commands(run_body):
    """Each logical shell command that files or edits an issue.

    Continuations are joined, because a delivery is routinely written across several lines
    and a suppression at the end of it belongs to the whole command.
    """
    joined, buf = [], ""
    for line in run_body.splitlines():
        stripped = line.rstrip()
        if stripped.endswith("\\"):
            buf += stripped[:-1] + " "
            continue
        joined.append(buf + stripped)
        buf = ""
    if buf:
        joined.append(buf)
    return [c for c in joined if DELIVERS.search(c)]


def steps_of(doc):
    """Every step in the document, with its job name."""
    jobs = doc.get("jobs")
    if not isinstance(jobs, dict):
        return
    for job_name, job in jobs.items():
        if not isinstance(job, dict):
            continue
        for step in job.get("steps") or []:
            if isinstance(step, dict):
                yield job_name, job, step


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--workflows", default=".github/workflows")
    args = ap.parse_args()

    workflows = Path(args.workflows)
    if not workflows.is_dir():
        print(f"::error::workflow directory not found: {workflows}", file=sys.stderr)
        return EXIT_ERROR

    files = sorted(list(workflows.glob("*.yml")) + list(workflows.glob("*.yaml")),
                   key=lambda p: p.name)
    if not files:
        print(f"::error::no workflow files under {workflows}", file=sys.stderr)
        return EXIT_ERROR

    delivery_steps = 0
    failures = []

    # --- assertion 2 runs over raw text, because the point is that position does not
    # --- matter: a comment, a run: block and a key are all evaluated alike.
    for path in files:
        text = path.read_text()
        for lineno, line in enumerate(text.splitlines(), start=1):
            if EMPTY_EXPRESSION.search(line):
                failures.append((path.name, lineno,
                                 "empty GitHub expression: the workflow is rejected before "
                                 "job creation, and the run is recorded under its file path "
                                 "rather than its name, so nothing matching on name sees it"))

    # --- assertion 1 needs structure, to tie a suppression to the step it sits on.
    for path in files:
        text = path.read_text()
        try:
            doc = yaml.safe_load(text)
        except yaml.YAMLError as e:
            print(f"::error file={path}::not valid YAML: {e}", file=sys.stderr)
            return EXIT_ERROR
        if not isinstance(doc, dict):
            continue

        for job_name, job, step in steps_of(doc):
            body = "\n".join(str(step.get(k, "")) for k in ("run", "with", "uses"))
            if not DELIVERS.search(body):
                continue
            delivery_steps += 1
            label = step.get("name") or f"(unnamed step in {job_name})"

            # The suppression must sit on the DELIVERY COMMAND, not merely somewhere in
            # the step. A step that files an issue may legitimately contain `|| true` on an
            # unrelated read -- `grep -c . || true` counts lines and returns 1 for zero,
            # which is a real answer handled by the assertion that follows it. Flagging
            # those would make the check cry wolf on correct code, and a check that cries
            # wolf is one people learn to override.
            #
            # STATED LIMITATION: a suppression elsewhere in a delivery step is not flagged.
            # The two `gh label create ... 2>/dev/null || true` calls in this repository are
            # real swallowed errors and this check does not catch them, because they are not
            # the delivery.
            for command in delivery_commands(step.get("run", "") or ""):
                for pattern, description in SUPPRESSIONS:
                    if pattern.search(command):
                        failures.append((path.name, None,
                                         f'step "{label}" applies `{description}` to the '
                                         f"command that files or edits the issue, so a "
                                         f"failure to deliver would not reach the run's "
                                         f"conclusion"))
            if step.get("continue-on-error") is True:
                failures.append((path.name, None,
                                 f'step "{label}" files or edits an issue and declares '
                                 f"`continue-on-error: true`"))
            if job.get("continue-on-error") is True:
                failures.append((path.name, None,
                                 f'job "{job_name}" contains an issue-filing step and '
                                 f"declares `continue-on-error: true`"))

    for name, lineno, reason in failures:
        loc = f"file={workflows.as_posix()}/{name}" + (f",line={lineno}" if lineno else "")
        print(f"::error {loc}::{reason}", file=sys.stderr)

    print(f"Escalation hygiene: {len(files)} workflow file(s) scanned, "
          f"{delivery_steps} issue-filing step(s) found, "
          f"{len([f for f in failures if f[1] is None])} suppression(s), "
          f"{len([f for f in failures if f[1] is not None])} empty expression(s).")

    # Zero-floor on the subject of assertion 1. Finding no delivery steps means the
    # enumeration is broken, not that the repository files no issues.
    if delivery_steps == 0:
        print("::error::found no issue-filing steps at all; the enumeration examined "
              "nothing and cannot have verified anything", file=sys.stderr)
        return EXIT_FAIL

    return EXIT_FAIL if failures else EXIT_PASS


if __name__ == "__main__":
    sys.exit(main())
