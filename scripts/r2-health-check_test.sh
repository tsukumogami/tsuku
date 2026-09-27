#!/usr/bin/env bash
# Tests for scripts/r2-health-check.sh.
#
# The health check exists to say whether R2 answered, and how fast. Its verdict must track
# the storage request, not the cost of starting whatever program makes the request. The
# first case below is the one that matters most: every client program on PATH is made to
# take 2.5 s to start, while the server answers at once. A check that times the process
# reports that as degraded; a check that times the request reports it healthy.
#
# The server is a local HTTP listener that stands in for R2. It also rejects any request
# that is not SigV4-signed with the credentials the test supplied, so a check that stopped
# signing its request would fail every case here rather than pass them.
#
# Usage: scripts/r2-health-check_test.sh
# Exit:  0 all cases passed, 1 a case failed, 2 the harness could not run (VOID).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECK="$SCRIPT_DIR/r2-health-check.sh"

for tool in python3 curl; do
  if ! command -v "$tool" >/dev/null; then
    echo "VOID: $tool is not installed, so no case could run" >&2
    exit 2
  fi
done

WORK=$(mktemp -d)
SERVER_PID=""
cleanup() {
  if [ -n "$SERVER_PID" ]; then kill "$SERVER_PID" 2>/dev/null || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT

ACCESS_KEY="TESTACCESSKEY"
SECRET_KEY="test-secret-key"
BUCKET="test-bucket"
MODE_FILE="$WORK/mode"
echo ok > "$MODE_FILE"

# --- the stand-in for R2 ------------------------------------------------------------
#
# The mode file is read on every request, so one server serves every case:
#   ok        200 at once
#   slow:<s>  200 after <s> seconds
#   status:<n> reply <n> at once
cat > "$WORK/server.py" <<'PY'
import http.server, sys, time

mode_file, access_key, bucket, port_file = sys.argv[1:5]

class Handler(http.server.BaseHTTPRequestHandler):
    def do_HEAD(self):
        mode = open(mode_file).read().strip()
        auth = self.headers.get("Authorization", "")
        signed = (auth.startswith("AWS4-HMAC-SHA256 ")
                  and f"Credential={access_key}/" in auth
                  and self.headers.get("x-amz-content-sha256")
                  and self.headers.get("x-amz-date"))
        if self.path != f"/{bucket}/health/ping.json" or not signed:
            self.send_response(403); self.send_header("Content-Length", "0"); self.end_headers()
            return
        if mode.startswith("slow:"):
            time.sleep(float(mode.split(":", 1)[1]))
        code = int(mode.split(":", 1)[1]) if mode.startswith("status:") else 200
        self.send_response(code); self.send_header("Content-Length", "2"); self.end_headers()

    def log_message(self, *args):
        pass

server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
with open(port_file, "w") as f:
    f.write(str(server.server_address[1]))
server.serve_forever()
PY

python3 "$WORK/server.py" "$MODE_FILE" "$ACCESS_KEY" "$BUCKET" "$WORK/port" &
SERVER_PID=$!
for _ in $(seq 1 50); do
  [ -s "$WORK/port" ] && break
  sleep 0.1
done
if [ ! -s "$WORK/port" ]; then
  echo "VOID: the stand-in server did not start, so no case could run" >&2
  exit 2
fi
PORT=$(cat "$WORK/port")

# --- slow-starting clients ------------------------------------------------------------
#
# Every program the check might use to reach R2 is wrapped so it takes 2.5 s to start and
# then behaves normally. `aws` has no real counterpart here, so its wrapper simply succeeds
# after the delay, which is what a healthy head-object does.
STARTUP_DELAY=2.5
SLOW_BIN="$WORK/slow-bin"
mkdir -p "$SLOW_BIN"
REAL_CURL=$(command -v curl)
printf '#!/usr/bin/env bash\nsleep %s\nexec %q "$@"\n' "$STARTUP_DELAY" "$REAL_CURL" > "$SLOW_BIN/curl"
printf '#!/usr/bin/env bash\nsleep %s\necho 2\n' "$STARTUP_DELAY" > "$SLOW_BIN/aws"
chmod +x "$SLOW_BIN/curl" "$SLOW_BIN/aws"

# --- harness --------------------------------------------------------------------------

FAILED=0
PASSED=0
OUT=""
ALL_OUT=""
RC=0

# run_check <path-prefix> [VAR=value ...]  -- runs the check, sets OUT and RC.
run_check() {
  local path_prefix="$1"; shift
  set +e
  OUT=$(env PATH="$path_prefix$PATH" \
      R2_BUCKET_URL="http://127.0.0.1:$PORT" \
      R2_BUCKET_NAME="$BUCKET" \
      R2_ACCESS_KEY_ID="$ACCESS_KEY" \
      R2_SECRET_ACCESS_KEY="$SECRET_KEY" \
      "$@" "$CHECK" 2>&1)
  RC=$?
  set -e
  ALL_OUT+="$OUT"$'\n'
}

pass() { PASSED=$((PASSED + 1)); echo "PASS: $1"; }
fail() { FAILED=$((FAILED + 1)); echo "FAIL: $1"; printf '%s\n' "$OUT" | sed 's/^/    /'; }

latency_ms() { printf '%s\n' "$OUT" | sed -n 's/^Latency: \([0-9][0-9]*\)ms$/\1/p' | head -1; }

# 1. Slow client start, fast server. The verdict must be healthy, and the latency it
#    reports must not include the 2.5 s start-up.
echo ok > "$MODE_FILE"
run_check "$SLOW_BIN:"
lat=$(latency_ms)
if [ "$RC" -eq 0 ] && grep -qx 'Status: healthy' <<<"$OUT" && [ -n "$lat" ] && [ "$lat" -lt 1000 ]; then
  pass "a slow-starting client does not make a fast R2 read as degraded (latency ${lat}ms)"
else
  fail "a slow-starting client made a fast R2 read as degraded (exit $RC, latency ${lat:-none})"
fi

# 2. Fast client, slow server. The request itself took 2.5 s, which is over the threshold.
echo slow:2.5 > "$MODE_FILE"
run_check ""
lat=$(latency_ms)
if [ "$RC" -eq 2 ] && grep -qx 'Status: degraded' <<<"$OUT" && [ -n "$lat" ] && [ "$lat" -ge 2500 ] && [ "$lat" -lt 4000 ]; then
  pass "a slow R2 response is degraded, and the reported latency is the response time (${lat}ms)"
else
  fail "a slow R2 response was not reported as degraded with its own latency (exit $RC, latency ${lat:-none})"
fi

# 3. The report says what it measured: connection and first-byte times are printed
#    separately from the total, so a slow handshake can be told from a slow response.
#    Reuses case 2's output: the server delayed the response, not the connection.
connect=$(printf '%s\n' "$OUT" | sed -n 's/^Connect: \([0-9][0-9]*\)ms$/\1/p')
first=$(printf '%s\n' "$OUT" | sed -n 's/^First byte: \([0-9][0-9]*\)ms$/\1/p')
if [ -n "$connect" ] && [ -n "$first" ] && [ "$connect" -lt 1000 ] && [ "$first" -ge 2500 ]; then
  pass "connect time (${connect}ms) and first-byte time (${first}ms) are reported separately"
else
  fail "connect and first-byte times are missing or do not match the delay (connect ${connect:-none}, first byte ${first:-none})"
fi

# 4. R2 answers but refuses: a non-200 is a failure, not a slow success.
echo status:403 > "$MODE_FILE"
run_check ""
if [ "$RC" -eq 1 ] && grep -qx 'Status: failure' <<<"$OUT" && grep -q 'HTTP 403' <<<"$OUT"; then
  pass "a 403 from R2 is reported as failure, naming the status"
else
  fail "a 403 from R2 was not reported as failure naming the status (exit $RC)"
fi

# 5. R2 does not answer in time: the 5 s timeout applies to the request.
echo slow:8 > "$MODE_FILE"
start=$(date +%s)
run_check ""
elapsed=$(( $(date +%s) - start ))
if [ "$RC" -eq 1 ] && grep -qx 'Status: failure' <<<"$OUT" && [ "$elapsed" -lt 8 ]; then
  pass "a request that exceeds the timeout is a failure, and the check gives up (${elapsed}s)"
else
  fail "a request that exceeds the timeout was not a timely failure (exit $RC, ${elapsed}s)"
fi

# 6. Nothing listening: a refused connection is a failure.
echo ok > "$MODE_FILE"
run_check "" R2_BUCKET_URL="http://127.0.0.1:1"
if [ "$RC" -eq 1 ] && grep -qx 'Status: failure' <<<"$OUT"; then
  pass "an unreachable endpoint is a failure"
else
  fail "an unreachable endpoint was not a failure (exit $RC)"
fi

# 7. Wrong credentials are refused by the stand-in, so the check must not report healthy.
#    This is what keeps the other cases honest about signing.
run_check "" R2_ACCESS_KEY_ID="SOMEONEELSE"
if [ "$RC" -eq 1 ]; then
  pass "a request signed with other credentials is refused and reported as failure"
else
  fail "a request signed with other credentials was not reported as failure (exit $RC)"
fi

# 8. Secrets never appear in the check's output, in any case above.
if [ -n "$ALL_OUT" ] && ! grep -q "$SECRET_KEY" <<<"$ALL_OUT"; then
  pass "the secret key does not appear in the output"
else
  fail "the secret key appears in the output"
fi

# 9. Missing configuration is a failure before any request is made.
run_check "" R2_BUCKET_URL=""
if [ "$RC" -eq 1 ] && grep -q 'R2_BUCKET_URL' <<<"$OUT"; then
  pass "a missing R2_BUCKET_URL is refused"
else
  fail "a missing R2_BUCKET_URL was not refused (exit $RC)"
fi

echo
echo "$PASSED passed, $FAILED failed"
[ "$FAILED" -eq 0 ]
