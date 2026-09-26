#!/usr/bin/env python3
"""Stub `gh` for escalation-backstop_test.py.

Answers `gh api [--paginate] PATH --jq EXPR` from the fixture named by $STUB_FIXTURE, the way
the API would: runs are filtered by the query's own branch, created, status and event
parameters; issues by creator, state and since; comments by since. The caller's --jq is
applied with real jq. So a check that drops a query parameter gets the unfiltered answer,
and its row fails, instead of passing because the fixture happened to be pre-filtered.

Failure injection: fixture "fail" maps a path substring to a count of failures before the
call succeeds, or -1 to fail every time. Counts persist in $STUB_FIXTURE.state.
"""
import json, os, subprocess, sys
from urllib.parse import urlsplit, parse_qs

args = sys.argv[1:]
if not args or args[0] != "api":
    sys.exit(f"stub gh: unsupported: {args}")
args = [a for a in args[1:] if a != "--paginate"]
path, jq = args[0], args[args.index("--jq") + 1]
fx_path = os.environ["STUB_FIXTURE"]
fx = json.load(open(fx_path))

with open(fx_path + ".log", "a") as log:
    log.write(path + "\n")

state_path = fx_path + ".state"
state = json.load(open(state_path)) if os.path.exists(state_path) else {}
for needle, count in fx.get("fail", {}).items():
    if needle in path:
        used = state.get(needle, 0)
        if count == -1 or used < count:
            state[needle] = used + 1
            json.dump(state, open(state_path, "w"))
            sys.stderr.write("HTTP 502: Bad Gateway (stub)\n")
            sys.exit(1)

u = urlsplit(path)
q = {k: v[0] for k, v in parse_qs(u.query).items()}
p = u.path
repo = "repos/o/r"

def since_ok(obj):
    return "since" not in q or obj.get("updated_at", "") >= q["since"]

if p == repo:
    doc = {"default_branch": fx.get("default_branch", "main")}
elif p.startswith(repo + "/actions/workflows/"):
    wf = p.split("/")[5]
    runs = fx.get("sweeps", []) if wf == "escalate-sweep.yml" else fx.get("runs", {}).get(wf, [])
    out = []
    for r in runs:
        if "branch" in q and r["head_branch"] != q["branch"]: continue
        if "status" in q and r.get("conclusion") != q["status"]: continue
        if "event" in q and r["event"] != q["event"]: continue
        if "created" in q and r["created_at"][:10] < q["created"].lstrip(">="): continue
        out.append(r)
    out.sort(key=lambda r: r["created_at"], reverse=True)
    if "per_page" in q: out = out[: int(q["per_page"])]
    doc = {"workflow_runs": out}
elif p == "search/issues":
    # Fuzzy, like the real index: any issue whose title CONTAINS the quoted phrase.
    phrase = q["q"].split('"')[1]
    doc = {"items": [i for i in fx.get("issues", []) if phrase in i["title"]]}
elif p == repo + "/issues":
    out = []
    for i in fx.get("issues", []):
        if q.get("creator") == "app/github-actions" and i["user"]["login"] != "github-actions[bot]": continue
        if q.get("state") == "open" and i.get("closed_at"): continue
        if q.get("state") == "closed" and not i.get("closed_at"): continue
        if not since_ok(i): continue
        out.append(i)
    doc = out
elif p == repo + "/issues/comments":
    doc = [c for c in fx.get("comments", []) if since_ok(c)]
elif p.startswith(repo + "/issues/"):
    n = int(p.rsplit("/", 1)[1])
    doc = next((i for i in fx.get("issues", []) if i["number"] == n), None)
    if doc is None:
        sys.stderr.write("HTTP 404: Not Found (stub)\n"); sys.exit(1)
else:
    sys.exit(f"stub gh: no fixture route for {path}")

proc = subprocess.run(["jq", "-r", jq], input=json.dumps(doc), capture_output=True, text=True)
sys.stdout.write(proc.stdout); sys.stderr.write(proc.stderr)
sys.exit(proc.returncode)
