#!/usr/bin/env bash
# Smoke test for a built Gotenberg image (Chromium variant). Usage: test.sh <image-ref> [version]
# Converts an HTML page to PDF through Chromium, merges the result with itself through
# the PDF engines, and checks /health reports Chromium up.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-gotenberg-smoke-$$"
DIR="$(mktemp -d)"
API=http://127.0.0.1:13000

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$DIR"; }
trap cleanup EXIT

docker run -d --name "$NAME" -p 127.0.0.1:13000:3000 "$IMAGE" >/dev/null
for i in $(seq 1 60); do
  curl -fsS "$API/health" >/dev/null 2>&1 && break
  [ "$i" = 60 ] && { echo "API never became healthy"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done

ver="$(curl -fsS "$API/version")"
echo "reported version: $ver"
case "$ver" in ""|*snapshot*) echo "version not stamped"; exit 1 ;; esac
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected $WANT"; exit 1; }
grep -q '"chromium":{"status":"up"' <<<"$(curl -fsS "$API/health")" \
  || { echo "chromium not up: $(curl -fsS "$API/health")"; exit 1; }

printf '<html><body><h1 style="hyphens:auto">QuenchWorks gotenberg smoke</h1></body></html>' > "$DIR/index.html"
curl -fsS -o "$DIR/a.pdf" -F "files=@$DIR/index.html" "$API/forms/chromium/convert/html" \
  || { echo "HTML to PDF failed"; docker logs "$NAME" | tail -30; exit 1; }
[ "$(head -c 5 "$DIR/a.pdf")" = "%PDF-" ] || { echo "chromium did not return a PDF"; exit 1; }
echo "  chromium: HTML -> PDF ($(wc -c < "$DIR/a.pdf") bytes)"

cp "$DIR/a.pdf" "$DIR/b.pdf"
curl -fsS -o "$DIR/m.pdf" -F "files=@$DIR/a.pdf" -F "files=@$DIR/b.pdf" "$API/forms/pdfengines/merge" \
  || { echo "merge failed"; docker logs "$NAME" | tail -30; exit 1; }
[ "$(head -c 5 "$DIR/m.pdf")" = "%PDF-" ] || { echo "merge did not return a PDF"; exit 1; }
echo "  pdfengines: merged two PDFs"

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (version $ver, nonroot user: $user, HTML -> PDF, PDF merge)"
