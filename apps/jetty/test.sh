#!/usr/bin/env bash
# Smoke test for a built Jetty image. Usage: test.sh <image-ref> [version]
# Deploys a directory web app through the pre-built JETTY_BASE's web app deployer and
# requires Jetty to serve it, reporting its version in the Server header.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="quench-jetty-smoke-$$"
DIR="$(mktemp -d)"
URL=http://127.0.0.1:18080

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$DIR"; }
trap cleanup EXIT

mkdir -p "$DIR/hello/WEB-INF"
printf 'quenchworks-jetty-%s\n' "$$" > "$DIR/hello/index.html"
cat > "$DIR/hello/WEB-INF/web.xml" <<'XML'
<?xml version="1.0" encoding="UTF-8"?>
<web-app xmlns="https://jakarta.ee/xml/ns/jakartaee" version="6.0"/>
XML
chmod -R a+rX "$DIR"

docker run -d --name "$NAME" -p 127.0.0.1:18080:8080 -v "$DIR:/var/lib/jetty/webapps:ro" "$IMAGE" >/dev/null
body=""
for i in $(seq 1 60); do
  body="$(curl -fsS "$URL/hello/" 2>/dev/null)" && break
  [ "$i" = 60 ] && { echo "the web app was never served"; docker logs "$NAME" | tail -30; exit 1; }
  sleep 1
done
[ "$body" = "quenchworks-jetty-$$" ] || { echo "unexpected body: $body"; exit 1; }
echo "  deployed webapps/hello and served /hello/"

server="$(curl -fsSI "$URL/hello/" | tr -d '\r' | awk -F': ' 'tolower($1)=="server" {print $2}')"
echo "Server header: $server"
case "$server" in Jetty\(*\)) ;; *) echo "not Jetty: '$server'"; exit 1 ;; esac
[ -z "$WANT" ] || [ "$server" = "Jetty($WANT)" ] || { echo "expected Jetty($WANT)"; exit 1; }

code="$(curl -s -o /dev/null -w '%{http_code}' "$URL/nope/")"
[ "$code" = 404 ] || { echo "expected 404 for an unknown context, got $code"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed ($server, nonroot user: $user, directory web app deployed and served)"
