#!/usr/bin/env bash
# Smoke test for a built trivy-scanner image. Usage: test.sh <image-ref> [version]
# Runs the same shape of command line Trivy Operator's scan Jobs use: /bin/sh -c with
# trivy writing a report to a file, then cat / bzip2 / base64 of it.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm "$IMAGE" --version 2>&1 | head -1)"
echo "$ver"
[ -z "$WANT" ] || echo "$ver" | grep -q "Version: ${WANT}" || { echo "expected version $WANT"; exit 1; }

out="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /bin/sh "$IMAGE" -c '
  mkdir -p /tmp/scan /tmp/fs/etc && echo "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" > /tmp/fs/etc/creds
  trivy fs --scanners secret --format json --output /tmp/scan/result.json /tmp/fs 2>/tmp/scan/result.json.log
  rc=$?; if [ $rc -eq 1 ]; then cat /tmp/scan/result.json.log; else bzip2 -c /tmp/scan/result.json | base64; fi; exit $rc
')"
echo "$out" | base64 -d | bzip2 -dc > /tmp/scanner-smoke.$$.json
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); s=[x for r in d.get("Results",[]) for x in r.get("Secrets") or []]; print("secrets found:", len(s)); sys.exit(0 if s else 1)' /tmp/scanner-smoke.$$.json \
  || { rm -f /tmp/scanner-smoke.$$.json; echo "the planted secret was not found"; exit 1; }
rm -f /tmp/scanner-smoke.$$.json
echo "smoke test passed (trivy-scanner ${WANT:-?}, uid $user, sh + trivy + bzip2 + base64 pipeline)"
