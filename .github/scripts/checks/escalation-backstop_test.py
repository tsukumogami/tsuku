#!/usr/bin/env python3
"""Regression cases for escalation-backstop.py (#2607).

Each case builds, on purpose, a state the check will meet: a failure that was told to
someone and one that wasn't, an item filed late and one closed before the run, a workflow
that recovered, a run no sweep has seen yet, a sweeper that stopped, a flaky API and a dead
one, a branch run, a pull-request run of a schedule-scoped workflow, an owner, a
self-filer, a rejected run, and a pull request that tries to exempt a workflow in its own
diff.

The rows assert what the check SAYS, not only its exit code: the fixed first line that
tells a reader this is about the repository and not their pull request, and the receipt's
counts against the truth each case constructed. A check whose account of itself drifted
would fail here while its verdicts still looked right.

`gh` is a stub (testdata/backstop-gh-stub.py) that filters by the query's own parameters,
so dropping a filter from the check changes what it receives. Declarations come from a
sandbox git repository, read at a ref, so the exemption case is real.
"""
import json, os, shutil, subprocess, sys, tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
CHECK = HERE / "escalation-backstop.py"
STUB = HERE / "testdata" / "backstop-gh-stub.py"
NOW = "2026-10-01T12:00:00Z"
SWEEP_AT = "2026-10-01T10:00:00Z"        # last completed scheduled sweep, 2h before NOW
HEADER = "Escalation backstop (repository health):"

passed = failed = 0


def report(ok, desc, detail=""):
    global passed, failed
    if ok:
        passed += 1
        print(f"  [PASS] {desc}")
    else:
        failed += 1
        print(f"  [FAIL] {desc} -- {detail}", file=sys.stderr)


WORKFLOWS = {
    "demo.yml": "# escalation-policy: issue\n# escalation-assignee: someone\n# coverage: items\nname: Demo\non:\n  schedule:\n    - cron: '0 * * * *'\n",
    "dual.yml": "# escalation-policy: issue\n# escalation-assignee: someone\n# escalation-only-on: schedule\n# coverage: items\nname: Dual\non:\n  schedule:\n    - cron: '0 * * * *'\n  pull_request:\n",
    "owned.yml": "# escalation-policy: issue\n# escalation-assignee: someone\n# escalation-owned-by: 500\n# escalation-owned-by-reason: fixture\n# coverage: items\nname: Owned\non:\n  schedule:\n    - cron: '0 * * * *'\n",
    "selffile.yml": "# escalation-policy: issue\n# escalation-assignee: someone\n# escalation-self-files: 600\n# escalation-self-files-note: fixture\n# coverage: items\nname: Selffile\non:\n  schedule:\n    - cron: '0 * * * *'\n",
    "quiet.yml": "# escalation-policy: none\n# escalation-reason: fixture\n# escalation-none-kind: permanent\nname: Quiet\non:\n  pull_request:\n",
}
REGISTRY = 'registered:\n  - "Demo"\n  - "Dual"\n  - "Owned"\n  - "Selffile"\n'


def make_repo(tmp):
    repo = tmp / "repo"
    (repo / ".github" / "workflows").mkdir(parents=True)
    for name, text in WORKFLOWS.items():
        (repo / ".github" / "workflows" / name).write_text(text)
    (repo / ".github" / "escalation-registry.yml").write_text(REGISTRY)
    env = dict(os.environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@t", GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@t")
    for cmd in (["git", "init", "-q", "-b", "main"], ["git", "add", "-A"], ["git", "commit", "-q", "-m", "main"]):
        subprocess.run(cmd, cwd=repo, check=True, env=env)
    return repo


def run(n, created, conclusion="success", event="schedule", branch="main", name=None, updated=None):
    return {"id": n, "name": name or "x", "event": event, "status": "completed", "conclusion": conclusion,
            "created_at": created, "updated_at": updated or created, "head_branch": branch,
            "html_url": f"https://github.com/o/r/actions/runs/{n}"}


def issue(n, title, created, closed=None, body="", login="github-actions[bot]"):
    return {"number": n, "title": title, "body": body, "created_at": created, "closed_at": closed,
            "updated_at": closed or created, "html_url": f"https://github.com/o/r/issues/{n}",
            "user": {"login": login}, "pull_request": None}


def base_fixture():
    """Healthy: every workflow has a green run, and nothing is failing."""
    return {
        "default_branch": "main",
        "sweeps": [{"created_at": SWEEP_AT, "conclusion": "success", "event": "schedule", "head_branch": "main"}],
        "runs": {
            "demo.yml": [run(1, "2026-09-30T00:00:00Z")],
            "dual.yml": [run(2, "2026-09-30T00:00:00Z")],
            "owned.yml": [run(3, "2026-09-30T00:00:00Z")],
            "selffile.yml": [run(4, "2026-09-30T00:00:00Z")],
        },
        "issues": [], "comments": [], "fail": {},
    }


def check(tmp, repo, fx, ref="main", extra=()):
    fx_path = tmp / "fixture.json"
    for suffix in ("", ".state", ".log"):
        p = Path(str(fx_path) + suffix)
        if p.exists():
            p.unlink()
    fx_path.write_text(json.dumps(fx))
    bindir = tmp / "bin"
    bindir.mkdir(exist_ok=True)
    gh = bindir / "gh"
    gh.write_text(f"#!/usr/bin/env bash\nexec python3 {STUB} \"$@\"\n")
    gh.chmod(0o755)
    env = dict(os.environ, PATH=f"{bindir}:{os.environ['PATH']}", STUB_FIXTURE=str(fx_path),
               BACKSTOP_RETRY_DELAY="0", BACKSTOP_ATTEMPTS="3", GITHUB_REPOSITORY="o/r")
    proc = subprocess.run([sys.executable, str(CHECK), "--ref", ref, "--now", NOW, *extra],
                          cwd=repo, env=env, capture_output=True, text=True)
    out = proc.stdout + proc.stderr
    return proc.returncode, out, out.splitlines()[0] if out.strip() else ""


def expect(desc, got, want_rc, first_prefix, *needles):
    rc, out, first = got
    problems = []
    if rc != want_rc:
        problems.append(f"exit {rc}, wanted {want_rc}")
    if not first.startswith(first_prefix):
        problems.append(f"first line {first!r}")
    for n in needles:
        if n not in out:
            problems.append(f"missing {n!r}")
    report(not problems, desc, "; ".join(problems) + ("\n" + out if problems else ""))


OK = f"{HEADER} OK -- "
FINDING = f"{HEADER} FINDING, not a fault in this pull request -- "
CANNOT = f"{HEADER} COULD NOT CHECK, not a finding and not a fault in this pull request -- "


def main():
    tmp = Path(tempfile.mkdtemp())
    try:
        repo = make_repo(tmp)
        print("escalation-backstop.py")

        fx = base_fixture()
        expect("a healthy repository passes, and its first line and receipt say what it examined",
               check(tmp, repo, fx), 0, OK,
               "Receipt: examined 4 run(s) of 4 registered workflow(s) on main since 2026-09-24 12:00Z: "
               "0 non-success, 0 recovered, 0 pending, 0 tracked, 0 untracked.",
               "Exempt by declaration (escalation-policy: none): Quiet (permanent)")

        fx = base_fixture()
        fx["runs"]["demo.yml"].append(run(10, "2026-09-30T06:00:00Z", "failure"))
        got = check(tmp, repo, fx)
        expect("a failure nobody was told about is a finding that says it is not this pull request's fault",
               got, 1, FINDING + "untracked: Demo run 10",
               "1 untracked", "::error title=Repository health, not this pull request::",
               'file an issue titled "Scheduled workflow failing: Demo"', "-f window_hours=31")

        fx["issues"] = [issue(700, "Scheduled workflow failing: Demo", "2026-09-30T06:00:20Z")]
        expect("an open escalator item created after the run tracks it", check(tmp, repo, fx), 0, OK,
               "1 tracked, 0 untracked", "Demo run 10 (schedule, failure, completed 2026-09-30T06:00:00Z) -- item #700")

        fx["issues"] = [issue(700, "Scheduled workflow failing: Demo", "2026-10-01T09:00:00Z")]
        expect("an item filed LATE still tracks the run, so a person can clear the check by filing it",
               check(tmp, repo, fx), 0, OK, "item #700")

        fx["issues"] = [issue(700, "Scheduled workflow failing: Demo", "2026-09-01T00:00:00Z", closed="2026-09-29T00:00:00Z")]
        expect("an item closed before the run completed does not track it", check(tmp, repo, fx), 1,
               FINDING + "untracked: Demo run 10")

        fx["issues"] = [issue(700, "Scheduled workflow failing: Demo", "2026-09-30T06:00:20Z", closed="2026-09-30T12:00:00Z")]
        expect("an item closed after the run completed tracks it", check(tmp, repo, fx), 0, OK, "item #700")

        fx["issues"] = [issue(700, "[DEMONSTRATION] Scheduled workflow failing: Demo", "2026-09-30T06:00:20Z")]
        expect("a title that merely contains the item's title does not track it", check(tmp, repo, fx), 1,
               FINDING + "untracked: Demo run 10")

        fx["issues"] = [issue(700, "Scheduled workflow failing: Demo", "2026-09-30T06:00:20Z", login="somebody")]
        expect("an item a PERSON filed under the exact title tracks the run: filing it is the remedy offered",
               check(tmp, repo, fx), 0, OK, "item #700")
        fx["issues"] = [issue(700, "Scheduled workflow failing: Demo (again)", "2026-09-30T06:00:20Z", login="somebody")]
        expect("a person's issue whose title only contains the item's title does not", check(tmp, repo, fx), 1,
               FINDING + "untracked: Demo run 10")

        fx = base_fixture()
        fx["runs"]["demo.yml"] = [run(10, "2026-09-30T06:00:00Z", "failure"), run(11, "2026-09-30T07:00:00Z")]
        expect("a failure followed by a success in the same scope is recovered", check(tmp, repo, fx), 0, OK,
               "1 non-success, 1 recovered")

        fx = base_fixture()
        fx["runs"]["demo.yml"].append(run(12, "2026-10-01T10:30:00Z", "failure", updated="2026-10-01T10:40:00Z"))
        expect("a failure no sweep has seen yet is pending, not untracked", check(tmp, repo, fx), 0, OK,
               "1 pending, 0 tracked, 0 untracked")

        fx = base_fixture()
        fx["runs"]["demo.yml"].append(run(13, "2026-10-01T09:30:00Z", "failure", updated="2026-10-01T10:30:00Z"))
        expect("pending is measured from completion, not start: a run that began before the sweep but ended after it",
               check(tmp, repo, fx), 0, OK, "1 pending")

        fx = base_fixture()
        fx["runs"]["dual.yml"].append(run(14, "2026-09-30T06:00:00Z", "failure", event="pull_request"))
        expect("a pull-request failure of a schedule-scoped workflow is not examined", check(tmp, repo, fx), 0, OK,
               "examined 4 run(s)", "0 non-success")

        fx = base_fixture()
        fx["runs"]["demo.yml"].append(run(15, "2026-09-30T06:00:00Z", "failure", branch="feature"))
        expect("a failure on a branch is not examined (#2686)", check(tmp, repo, fx), 0, OK,
               "examined 4 run(s)", "0 non-success")

        fx = base_fixture()
        fx["runs"]["demo.yml"].append(run(16, "2026-09-20T06:00:00Z", "failure"))
        expect("a failure older than the window is not examined", check(tmp, repo, fx), 0, OK, "examined 4 run(s)")

        fx = base_fixture()
        fx["runs"]["owned.yml"].append(run(17, "2026-09-30T06:00:00Z", "failure"))
        fx["issues"] = [issue(500, "decision about Owned", "2026-09-01T00:00:00Z", login="dangazineu")]
        expect("a failure of an owned workflow is tracked by its open owner", check(tmp, repo, fx), 0, OK,
               "-- owner #500")
        fx["issues"] = [issue(500, "decision about Owned", "2026-09-01T00:00:00Z", closed="2026-09-29T00:00:00Z", login="dangazineu")]
        expect("an owner closed before the run does not track it", check(tmp, repo, fx), 1, FINDING + "untracked: Owned run 17")

        fx = base_fixture()
        fx["runs"]["selffile.yml"].append(run(18, "2026-09-30T06:00:00Z", "failure"))
        url = "https://github.com/o/r/actions/runs/18"
        fx["issues"] = [issue(601, "[Automated] Selffile failed", "2026-09-30T06:00:30Z", body=f"**Run:** {url}")]
        expect("a self-filing workflow's own bot issue carrying the run URL tracks it", check(tmp, repo, fx), 0, OK,
               "-- self-filed #601")
        fx["issues"] = [issue(601, "[Automated] Selffile failed", "2026-09-25T00:00:00Z", body="older run")]
        fx["comments"] = [{"user": {"login": "github-actions[bot]"}, "body": f"**Run:** {url}",
                           "issue_url": "https://api.github.com/repos/o/r/issues/601",
                           "created_at": "2026-09-30T06:00:30Z", "updated_at": "2026-09-30T06:00:30Z"}]
        expect("a bot comment carrying the run URL tracks it", check(tmp, repo, fx), 0, OK, "self-filed comment on #601")
        fx["comments"][0]["user"]["login"] = "somebody"
        expect("a person mentioning the run URL does not track it", check(tmp, repo, fx), 1,
               FINDING + "untracked: Selffile run 18")

        fx = base_fixture()
        fx["runs"]["demo.yml"].append(run(19, "2026-09-30T06:00:00Z", "failure", name=".github/workflows/demo.yml"))
        fx["issues"] = [issue(702, "Workflow rejected before job creation: .github/workflows/demo.yml", "2026-09-30T09:00:00Z")]
        expect("a run rejected before job creation is tracked by the rejected-run item", check(tmp, repo, fx), 0, OK,
               "item #702")

        fx = base_fixture()
        fx["sweeps"] = [{"created_at": "2026-09-30T11:00:00Z", "conclusion": "success", "event": "schedule", "head_branch": "main"}]
        expect("a sweeper whose last completed scheduled run is past 24h is a finding", check(tmp, repo, fx), 1,
               FINDING + "the last completed scheduled Escalate Sweep started 25.0h ago")
        fx["sweeps"][0]["created_at"] = "2026-09-30T12:01:00Z"
        expect("just inside the 24h limit it is not", check(tmp, repo, fx), 0, OK)
        fx["sweeps"] = [{"created_at": "2026-10-01T11:00:00Z", "conclusion": "success", "event": "workflow_dispatch", "head_branch": "main"},
                        {"created_at": "2026-09-29T00:00:00Z", "conclusion": "success", "event": "schedule", "head_branch": "main"}]
        expect("a recent MANUAL sweep does not stand in for the scheduled one", check(tmp, repo, fx), 1,
               FINDING + "the last completed scheduled Escalate Sweep started 60.0h ago")
        fx["sweeps"] = []
        expect("no scheduled sweep at all is a finding", check(tmp, repo, fx), 1,
               FINDING + "no completed scheduled Escalate Sweep found")

        fx = base_fixture()
        fx["runs"] = {k: [] for k in fx["runs"]}
        expect("examining nothing is a finding, not a pass", check(tmp, repo, fx), 1,
               FINDING + "examined no runs of 4 registered workflow(s)")

        fx = base_fixture()
        fx["runs"]["demo.yml"].append(run(10, "2026-09-30T06:00:00Z", "failure"))
        fx["fail"] = {"/actions/workflows/demo.yml/runs": 2}
        expect("a query that fails twice and then answers is retried, and the verdict is the real one",
               check(tmp, repo, fx), 1, FINDING + "untracked: Demo run 10")
        fx = base_fixture()
        fx["fail"] = {"/actions/workflows/demo.yml/runs": -1}
        got = check(tmp, repo, fx)
        expect("a query that keeps failing is COULD NOT CHECK, exit 3, never a finding", got, 3, CANNOT,
               "failed after 3 attempt(s): HTTP 502", "Re-running the job is the remedy")
        report("untracked" not in got[1], "and it accuses no workflow of being untracked", got[1])
        fx = base_fixture()
        fx["runs"]["demo.yml"].append(run(10, "2026-09-30T06:00:00Z", "failure"))
        fx["fail"] = {"creator=app/github-actions&state=open": -1}
        got = check(tmp, repo, fx)
        expect("the issue listing failing is COULD NOT CHECK too, not untracked", got, 3, CANNOT)

        # A pull request that exempts Demo in its own diff: the check reads main, not the head.
        env = dict(os.environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@t", GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@t")
        subprocess.run(["git", "checkout", "-q", "-b", "pr"], cwd=repo, check=True)
        wf = repo / ".github" / "workflows" / "demo.yml"
        wf.write_text(wf.read_text().replace("# coverage: items\n", "# escalation-owned-by: 999\n# escalation-owned-by-reason: x\n# coverage: items\n"))
        (repo / ".github" / "escalation-registry.yml").write_text(REGISTRY.replace('  - "Demo"\n', ""))
        subprocess.run(["git", "commit", "-qam", "exempt"], cwd=repo, check=True, env=env)
        fx = base_fixture()
        fx["runs"]["demo.yml"].append(run(10, "2026-09-30T06:00:00Z", "failure"))
        fx["issues"] = [issue(999, "anything", "2026-09-01T00:00:00Z", login="x")]
        expect("a pull request cannot exempt a workflow by editing its own declaration or the registry",
               check(tmp, repo, fx, ref="main"), 1, FINDING + "untracked: Demo run 10")
        expect("control: read at the pull request's head, that edit would have exempted it",
               check(tmp, repo, fx, ref="pr"), 0, OK)
        subprocess.run(["git", "checkout", "-q", "main"], cwd=repo, check=True)
    finally:
        shutil.rmtree(tmp)

    print(f"\nescalation-backstop self-test: {passed} passed, {failed} failed.")
    return 0 if failed == 0 and passed > 0 else 1


if __name__ == "__main__":
    sys.exit(main())
