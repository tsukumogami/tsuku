#!/usr/bin/env python3
"""Escalation backstop (#2607): was every recent failure of a registered workflow told to someone?

The listener and the sweeper share one script, one permission grant and one assignee
pre-flight, so a defect in any of those silences both at once. This check is the one
control independent of both. It runs from the pull-request lint workflow, which shares no
failure mode with cron and fires when a person is present. It never calls escalate.sh, it
holds no write scope, and it asks the Actions API itself rather than trusting the sweeper's
receipt.

For each registered workflow it lists the completed runs on the default branch inside the
window, scoped by `escalation-only-on`, and sorts every non-success run into one of:

  recovered  a later run in the same scope succeeded (the sweeper's own gate)
  pending    it completed after the last completed scheduled sweep started, so no sweep
             has had a chance to see it yet
  tracked    an issue that tracks the workflow was open at some point at or after the run
             completed: its escalator item (exact title), its declared owner, or -- for a
             self-filing workflow -- a bot issue or bot comment carrying the run's URL
  untracked  none of the above: nobody was told

The rule for "tracked" accepts an item filed late on purpose. A person who meets this
check red can clear it by filing the item, which is the outcome it exists to force. An
item closed before the run completed does not count, so a stale title match cannot
satisfy it (decision recorded on #2607).

Declarations and the registry are read from --ref, the default branch, not from the pull
request's head: a pull request must not be able to exempt a workflow by editing its own
diff.

This check's verdict is about the REPOSITORY, not the pull request it appears on. Its first
line of output always says so, in a fixed form, so a reader -- or a script -- can tell at a
glance that a red here is not the pull request's fault:

  Escalation backstop (repository health): OK -- ...
  Escalation backstop (repository health): FINDING, not a fault in this pull request -- ...
  Escalation backstop (repository health): COULD NOT CHECK, not a finding and not a fault in this pull request -- ...

Exit: 0 OK, 1 FINDING (an untracked run, a stale or missing sweep, or nothing examined),
3 COULD NOT CHECK (a query failed after retries, or the declarations could not be read).
A failed query is never reported as untracked.
"""

import argparse
import datetime as dt
import json
import os
import re
import subprocess
import sys
import time
import urllib.parse

HEADER = "Escalation backstop (repository health):"
BOT_LOGIN = "github-actions[bot]"
ITEM_TITLE = "Scheduled workflow failing: {}"
REJECTED_TITLE = "Workflow rejected before job creation: {}"
KEY = re.compile(r"^#\s*(?P<key>escalation-[a-z-]+):\s*(?P<value>.*?)\s*$")


class CouldNotCheck(Exception):
    pass


class Api:
    """`gh api` with retries. A failure after the last attempt is CouldNotCheck, never a finding."""

    def __init__(self, repo, attempts, delay):
        self.repo, self.attempts, self.delay = repo, attempts, delay

    def lines(self, path, jq, paginate=False):
        cmd = ["gh", "api"] + (["--paginate"] if paginate else []) + [path, "--jq", jq]
        err = ""
        for attempt in range(1, self.attempts + 1):
            proc = subprocess.run(cmd, capture_output=True, text=True)
            if proc.returncode == 0:
                return [line for line in proc.stdout.splitlines() if line.strip()]
            err = (proc.stderr or proc.stdout).strip().splitlines()[-1:] or ["exit %d" % proc.returncode]
            err = err[0]
            if attempt < self.attempts:
                time.sleep(self.delay * attempt)
        raise CouldNotCheck(f"`gh api {path}` failed after {self.attempts} attempt(s): {err}")

    def objects(self, path, jq, paginate=False):
        try:
            return [json.loads(line) for line in self.lines(path, jq + " | tojson", paginate)]
        except json.JSONDecodeError as e:
            raise CouldNotCheck(f"`gh api {path}` returned something that is not JSON: {e}")


def parse_time(s):
    return dt.datetime.fromisoformat(s.replace("Z", "+00:00")) if s else None


def git(*args):
    proc = subprocess.run(["git", *args], capture_output=True, text=True)
    if proc.returncode != 0:
        raise CouldNotCheck(f"`git {' '.join(args)}` failed: {proc.stderr.strip()}")
    return proc.stdout


def read_declarations(ref, workflows_dir, registry_path):
    """(registered names, {name: (file, declaration)}) as they stand on `ref`."""
    registry = git("show", f"{ref}:{registry_path}")
    registered = re.findall(r'^\s*-\s*"([^"]+)"\s*$', registry, re.M)
    if not registered:
        raise CouldNotCheck(f"{registry_path} on {ref} lists no workflows")
    files = [f for f in git("ls-tree", "--name-only", f"{ref}:{workflows_dir}").split()
             if f.endswith((".yml", ".yaml"))]
    by_name = {}
    for f in files:
        text = git("show", f"{ref}:{workflows_dir}/{f}")
        decl = {}
        for raw in text.splitlines():
            if not raw.startswith("#"):
                if raw.strip():
                    break
                continue
            m = KEY.match(raw)
            if m:
                decl.setdefault(m.group("key"), m.group("value"))
        m = re.search(r"^name:\s*['\"]?(.+?)['\"]?\s*$", text, re.M)
        if m:
            by_name[m.group(1)] = (f, decl)
    return registered, by_name


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--ref", required=True, help="git ref of the default branch to read declarations from")
    ap.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY", ""))
    ap.add_argument("--days", type=int, default=7, help="how far back to examine runs")
    ap.add_argument("--stale-hours", type=float, default=24.0,
                    help="fail when the last completed scheduled sweep is older than this")
    ap.add_argument("--now", default=None, help="ISO time to treat as now (tests)")
    ap.add_argument("--workflows-dir", default=".github/workflows")
    ap.add_argument("--registry", default=".github/escalation-registry.yml")
    args = ap.parse_args()

    api = Api(args.repo, int(os.environ.get("BACKSTOP_ATTEMPTS", "3")),
              float(os.environ.get("BACKSTOP_RETRY_DELAY", "5")))
    now = parse_time(args.now) if args.now else dt.datetime.now(dt.timezone.utc)
    start = now - dt.timedelta(days=args.days)
    try:
        return run(args, api, now, start)
    except CouldNotCheck as e:
        print(f"{HEADER} COULD NOT CHECK, not a finding and not a fault in this pull request -- {e}")
        print(f"::error title=Escalation backstop could not check::{e}. Nothing was found "
              f"wrong; the check could not look. Re-running the job is the remedy.")
        return 3


def run(args, api, now, start):
    registered, by_name = read_declarations(args.ref, args.workflows_dir, args.registry)
    default = api.lines(f"repos/{args.repo}", ".default_branch")
    if len(default) != 1:
        raise CouldNotCheck(f"repos/{args.repo} did not return one default branch")
    default = default[0]

    # The last completed scheduled sweep: what "pending" is measured against, and whose
    # age says whether the sweeper is alive at all.
    sweeps = api.objects(
        f"repos/{args.repo}/actions/workflows/escalate-sweep.yml/runs?status=success&event=schedule"
        f"&branch={default}&per_page=1", ".workflow_runs[] | {created_at}")
    last_sweep = parse_time(sweeps[0]["created_at"]) if sweeps else None

    counts = dict(examined=0, failing=0, recovered=0, pending=0, tracked=0, untracked=0)
    untracked, tracked_lines, exempt = [], [], []
    bot_issues = None      # fetched once, lazily
    bot_comments = None
    owners = {}
    searched = {}          # title -> search results, one search per title

    def load_bot_issues():
        nonlocal bot_issues
        if bot_issues is None:
            fields = "{number, title, body, created_at, closed_at, html_url, pr: (.pull_request != null)}"
            open_ = api.objects(f"repos/{args.repo}/issues?creator=app/github-actions&state=open&per_page=100",
                                f".[] | {fields}", paginate=True)
            # A closed issue that was open at or after a run in the window was updated
            # after the window started, so `since` loses nothing here.
            closed = api.objects(f"repos/{args.repo}/issues?creator=app/github-actions&state=closed"
                                 f"&since={start.strftime('%Y-%m-%dT%H:%M:%SZ')}&per_page=100",
                                 f".[] | {fields}", paginate=True)
            bot_issues = [i for i in open_ + closed if not i["pr"]]
        return bot_issues

    def load_bot_comments():
        nonlocal bot_comments
        if bot_comments is None:
            bot_comments = api.objects(
                f"repos/{args.repo}/issues/comments?since={start.strftime('%Y-%m-%dT%H:%M:%SZ')}&per_page=100",
                ".[] | {user: .user.login, body, issue_url, created_at}", paginate=True)
        return bot_comments

    def open_at_or_after(issue, completed):
        closed = parse_time(issue.get("closed_at"))
        return closed is None or closed >= completed

    for name, (f, decl) in sorted(by_name.items()):
        if decl.get("escalation-policy") == "none" and name not in registered:
            exempt.append(f"{name} ({decl.get('escalation-none-kind', 'none')})")

    for name in registered:
        if name not in by_name:
            raise CouldNotCheck(f"registered workflow \"{name}\" matches no workflow file on {args.ref}")
        f, decl = by_name[name]
        only_on = decl.get("escalation-only-on")
        runs = api.objects(
            f"repos/{args.repo}/actions/workflows/{f}/runs?branch={default}"
            f"&created=>={start.strftime('%Y-%m-%d')}&per_page=100",
            ".workflow_runs[] | {id, name, event, status, conclusion, created_at, updated_at, head_branch, html_url}",
            paginate=True)
        runs = [r for r in runs
                if r["status"] == "completed" and r["head_branch"] == default
                and parse_time(r["created_at"]) >= start
                and (not only_on or r["event"] == only_on)]
        counts["examined"] += len(runs)
        for r in runs:
            if r["conclusion"] == "success":
                continue
            counts["failing"] += 1
            completed = parse_time(r["updated_at"])
            label = f"{name} run {r['id']} ({r['event']}, {r['conclusion']}, completed {r['updated_at']})"
            if any(s["conclusion"] == "success" and parse_time(s["created_at"]) > parse_time(r["created_at"])
                   for s in runs):
                counts["recovered"] += 1
                continue
            if last_sweep is not None and completed > last_sweep:
                counts["pending"] += 1
                continue

            how = None
            titles = {ITEM_TITLE.format(name)}
            if r["name"].startswith(".github/"):
                titles.add(REJECTED_TITLE.format(r["name"]))
            for i in load_bot_issues():
                if i["title"] in titles and open_at_or_after(i, completed):
                    how = f"item #{i['number']}"
                    break
            if how is None:
                # The item may have been filed by a person -- which is the remedy this check
                # offers -- so look for the title from any author. Search is fuzzy, so the
                # match is still exact string equality on what comes back.
                for title in sorted(titles):
                    if title not in searched:
                        query = urllib.parse.quote(f'repo:{args.repo} is:issue in:title "{title}"')
                        searched[title] = api.objects(
                            f"search/issues?q={query}&per_page=100",
                            ".items[] | {number, title, created_at, closed_at}")
                    for i in searched[title]:
                        if i["title"] == title and open_at_or_after(i, completed):
                            how = f"item #{i['number']}"
                            break
                    if how:
                        break
            owner = decl.get("escalation-owned-by", "").lstrip("#")
            if how is None and owner:
                if owner not in owners:
                    got = api.objects(f"repos/{args.repo}/issues/{owner}", "{number, created_at, closed_at}")
                    owners[owner] = got[0] if got else None
                if owners[owner] and open_at_or_after(owners[owner], completed):
                    how = f"owner #{owner}"
            if how is None and decl.get("escalation-self-files"):
                for i in load_bot_issues():
                    if r["html_url"] in (i.get("body") or "") and open_at_or_after(i, completed):
                        how = f"self-filed #{i['number']}"
                        break
                if how is None:
                    for c in load_bot_comments():
                        if c["user"] == BOT_LOGIN and r["html_url"] in (c.get("body") or ""):
                            number = c["issue_url"].rsplit("/", 1)[-1]
                            issue = next((i for i in load_bot_issues() if str(i["number"]) == number), None)
                            if issue is None or open_at_or_after(issue, completed):
                                how = f"self-filed comment on #{number}"
                                break
            if how:
                counts["tracked"] += 1
                tracked_lines.append(f"  tracked: {label} -- {how}")
            else:
                counts["untracked"] += 1
                untracked.append((name, r, label))

    findings = []
    if counts["examined"] == 0:
        findings.append(f"examined no runs of {len(registered)} registered workflow(s) since "
                        f"{start:%Y-%m-%d %H:%MZ}; a check that looked at nothing has established nothing")
    if last_sweep is None:
        findings.append("no completed scheduled Escalate Sweep found on the default branch; "
                        "the sweeper is not running")
    else:
        age = (now - last_sweep).total_seconds() / 3600
        if age > args.stale_hours:
            findings.append(f"the last completed scheduled Escalate Sweep started {age:.1f}h ago "
                            f"({last_sweep:%Y-%m-%d %H:%MZ}), past the {args.stale_hours:g}h limit; "
                            f"the sweeper has stopped")
    for name, r, label in untracked:
        findings.append(f"untracked: {label} -- no issue for it was open at or after it completed")

    summary = (f"examined {counts['examined']} run(s) of {len(registered)} registered workflow(s) on "
               f"{default} since {start:%Y-%m-%d %H:%MZ}: {counts['failing']} non-success, "
               f"{counts['recovered']} recovered, {counts['pending']} pending, {counts['tracked']} tracked, "
               f"{counts['untracked']} untracked")
    if findings:
        print(f"{HEADER} FINDING, not a fault in this pull request -- {findings[0]}")
    else:
        print(f"{HEADER} OK -- {summary}")
    print(f"Receipt: {summary}.")
    print("Last completed scheduled sweep: " + (f"{last_sweep:%Y-%m-%d %H:%MZ}" if last_sweep else "none"))
    print("Exempt by declaration (escalation-policy: none): " + (", ".join(exempt) or "none"))
    for line in tracked_lines:
        print(line)
    for f in findings:
        print(f"::error title=Repository health, not this pull request::{f}")
    for name, r, label in untracked:
        # A dispatched sweep only looks back as far as its window, so name one that reaches
        # this run rather than leaving the default, which would not.
        hours = int((now - parse_time(r["created_at"])).total_seconds() // 3600) + 1
        print(f"  untracked: {label} {r['html_url']}")
        print(f"    to clear: file an issue titled \"{ITEM_TITLE.format(name)}\", or run "
              f"`gh workflow run escalate-sweep.yml -f window_hours={hours}`, which files it")
    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main())
