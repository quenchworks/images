#!/usr/bin/env bash
# Smoke test for a built terralist image. Usage: test.sh <image-ref> [version]
# Real registry work on a read-only root with /data on a volume: Terralist starts on
# SQLite with the local store (a dummy GitHub OAuth app, which it only calls at login),
# the master API key creates an authority, a module version is uploaded from a public
# archive, and the Terraform registry protocol lists it and serves its zip.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="terralist-smoke-$$"
WORK="$(mktemp -d "$PWD/.smoketest.XXXXXX")"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT
trap 'echo "failed at line $LINENO"; docker logs "$NAME" 2>&1 | tail -30' ERR

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" version)"
[ -z "$WANT" ] || echo "$ver" | grep -qF "$WANT" || { echo "version mismatch: $ver"; exit 1; }

KEY="mk-smoke-$$"
docker run -d --name "$NAME" --read-only --tmpfs /tmp --tmpfs /data:uid=1001,gid=1001 -p 127.0.0.1:15758:5758 "$IMAGE" server \
  --url http://127.0.0.1:15758 --sqlite-path /data/terralist.db \
  --local-store /data/store --local-registry /data/registry \
  --modules-storage-resolver local --providers-storage-resolver local \
  --token-signing-secret "tok-$RANDOM$RANDOM$RANDOM" --local-token-signing-secret "loc-$RANDOM$RANDOM$RANDOM" \
  --cookie-secret "$(printf 'c%.0s' $(seq 1 32))" --oauth-state-secret "st-$RANDOM$RANDOM$RANDOM" \
  --oauth-provider github --gh-client-id smoke --gh-client-secret smoke --master-api-key "$KEY" >/dev/null
B=http://127.0.0.1:15758
ok=0
for _ in $(seq 1 60); do curl -fsS "$B/.well-known/terraform.json" 2>/dev/null | grep -q 'modules.v1' && { ok=1; break; }; sleep 1; done
[ "$ok" = 1 ] || { echo "terralist never answered service discovery"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }

H="X-API-Key: $KEY"
curl -fsS -o /dev/null -H "$H" -H 'Content-Type: application/json' \
  -d '{"name":"quench","public":true,"owner":"smoke@example.com"}' "$B/v1/api/authorities/"
curl -fsS -H "$H" -H 'Content-Type: application/json' \
  -d '{"download_url":"https://github.com/cloudposse/terraform-null-label/archive/refs/tags/0.25.0.tar.gz"}' \
  "$B/v1/api/modules/quench/label/null/0.25.0/upload" | grep -q '"errors":\[\]'
curl -fsS "$B/v1/modules/quench/label/null/versions" | grep -q '"version":"0.25.0"'
get="$(curl -fsS -o /dev/null -D - "$B/v1/modules/quench/label/null/0.25.0/download" | tr -d '\r' | sed -n 's/^[Xx]-[Tt]erraform-[Gg]et: //p')"
[ -n "$get" ] || { echo "no X-Terraform-Get"; exit 1; }
curl -fsS -o "$WORK/m.zip" "$get"
unzip -l "$WORK/m.zip" | grep -q 'main.tf' || { echo "module zip has no main.tf"; exit 1; }
if docker logs "$NAME" 2>&1 | grep -E '"level":"(error|fatal|panic)"'; then echo "terralist logged errors"; exit 1; fi
echo "smoke test passed (terralist ${WANT:-?}, uid $user, module uploaded, listed and served)"
