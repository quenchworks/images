#!/usr/bin/env bash
# Smoke test for a built GoAccess image. Usage: test.sh <image-ref> [version]
# GoAccess parses a known combined-format access log (6 requests, 3 visitors, one 404) into a
# JSON report; the totals must match. It also renders an HTML report.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
v="$(docker run --rm "$IMAGE" --version 2>&1)"
echo "$v" | head -1
[ -z "$WANT" ] || grep -qF "GoAccess - $WANT" <<<"$v" || { echo "expected $WANT"; exit 1; }

cat > "$WORK/access.log" <<'LOG'
10.0.0.1 - - [12/Nov/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 512 "-" "Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0"
10.0.0.1 - - [12/Nov/2026:10:00:01 +0000] "GET /about HTTP/1.1" 200 256 "-" "Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0"
10.0.0.2 - - [12/Nov/2026:10:00:02 +0000] "GET / HTTP/1.1" 200 512 "-" "Mozilla/5.0 (Macintosh) Safari/605.1.15"
10.0.0.2 - - [12/Nov/2026:10:00:03 +0000] "GET /missing HTTP/1.1" 404 128 "-" "Mozilla/5.0 (Macintosh) Safari/605.1.15"
10.0.0.3 - - [12/Nov/2026:10:00:04 +0000] "GET / HTTP/1.1" 200 512 "-" "curl/8.0.0"
10.0.0.3 - - [12/Nov/2026:10:00:05 +0000] "POST /api HTTP/1.1" 201 64 "-" "curl/8.0.0"
LOG
chmod 0777 "$WORK"; chmod a+r "$WORK/access.log"
docker run --rm --read-only -v "$WORK:/w" "$IMAGE" /w/access.log --log-format=COMBINED -o /w/report.json >/dev/null 2>&1
docker run --rm --read-only -v "$WORK:/w" "$IMAGE" /w/access.log --log-format=COMBINED -o /w/report.html >/dev/null 2>&1
jq -r '.general | "total_requests \(.total_requests) unique_visitors \(.unique_visitors) failed \(.failed_requests)"' "$WORK/report.json"
jq -e '.general | .total_requests == 6 and .unique_visitors == 3 and .failed_requests == 0' "$WORK/report.json" >/dev/null || { echo "report counts wrong"; exit 1; }
grep -q '<html' "$WORK/report.html" || { echo "no HTML report"; exit 1; }
echo "smoke test passed: JSON totals match the log, HTML report rendered, user 1001"
