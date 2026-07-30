#!/usr/bin/env bash
# Smoke test for a built phpMyAdmin image. Usage: test.sh <image-ref>
#
# phpMyAdmin is a php-fpm app fronted by nginx (both under supervisord). It administers a
# database it does not own, so this test runs WITHOUT one and proves everything that does
# not need a live server:
#   * the image boots under a READ-ONLY rootfs with only the writable carve-out the chart
#     ships (tmpfs /tmp)
#   * php-fpm answers FastCGI and nginx serves HTTP 200 on / -- and the body is the REAL
#     rendered login form (the <input name="pma_username"> field), which means the Twig
#     template engine, the autoloader and the vendored tree all actually work. A TCP
#     connect (or a bare 200) would pass even with a broken PHP stack, so we assert markup.
#   * the login form is served over cookie auth, i.e. the baked config.inc.php parsed and
#     the blowfish_secret fallback worked (no "cookie auth needs blowfish_secret" error)
#   * the remediated twig/twig (>= 3.27.0, not the 3.11.3 the tarball vendors) is what is
#     installed, and the js-cookie prototype-pollution guard is present in the shipped asset
#   * PHP reports a version and every required extension is loaded
#   * the container runs as the nonroot phpmyadmin user (uid 1001)
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref>}"
NAME="quench-phpmyadmin-smoke-$$"
PORT=8097

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

# must be the nonroot phpmyadmin user (uid 1001)
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "starting $IMAGE (read-only rootfs, tmpfs /tmp)"
docker run -d --name "$NAME" \
  --read-only \
  --tmpfs /tmp:rw,mode=1777,uid=1001,gid=1001 \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" >/dev/null

out=/tmp/pma-smoke-$$.html
ok=""
for i in $(seq 1 60); do
  code="$(curl -fsS -o "$out" -w '%{http_code}' "http://127.0.0.1:${PORT}/" 2>/dev/null || true)"
  if [ "$code" = "200" ]; then ok=1; echo "GET / -> HTTP 200 (after ${i}s)"; break; fi
  if ! docker ps --format '{{.Names}}' | grep -q "^${NAME}$"; then
    echo "container exited early"; docker logs "$NAME"; exit 1
  fi
  sleep 1
done
[ -n "$ok" ] || { echo "phpMyAdmin did not serve HTTP 200 (last '$code')"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }

# The body must be the rendered login form, not an error page or a directory listing.
grep -q 'name="pma_username"' "$out" || {
  echo "body is not the phpMyAdmin login form"; head -60 "$out"; docker logs "$NAME" 2>&1 | tail -30; exit 1; }
grep -qi 'phpmyadmin' "$out" || { echo "body does not mention phpMyAdmin"; exit 1; }
# A missing/short blowfish_secret makes phpMyAdmin refuse cookie auth with this error.
grep -qi 'blowfish_secret' "$out" && {
  echo "login page complains about blowfish_secret"; exit 1; }
echo "login form rendered (Twig + autoloader + config.inc.php all live)"

# php version + the extensions phpMyAdmin requires/recommends.
echo -n "php: "; docker exec "$NAME" php --version | head -1
for ext in mysqli mysqlnd mbstring iconv openssl sodium zip bz2 gd curl ctype \
           fileinfo xml dom simplexml json hash pcre session zlib; do
  docker exec "$NAME" php -m | grep -qi "^${ext}$" \
    || { echo "PHP extension '$ext' missing"; docker exec "$NAME" php -m; exit 1; }
done
docker exec "$NAME" php -m | grep -qi opcache \
  || { echo "OPcache not loaded"; exit 1; }
echo "PHP extensions present (mysqli mysqlnd mbstring iconv openssl sodium zip bz2 gd curl ctype fileinfo xml dom simplexml + opcache)"

# Shipped version + the CVE remediations must both be in the image.
docker exec "$NAME" grep -q "public const VERSION = '" /var/www/html/libraries/classes/Version.php \
  || { echo "Version.php missing"; exit 1; }
docker exec "$NAME" php -r '$i=json_decode(file_get_contents("/var/www/html/vendor/composer/installed.json"),true);
  foreach($i["packages"] as $p){ if($p["name"]==="twig/twig"){ echo "twig ",$p["version"],"\n";
    exit(version_compare(ltrim($p["version"],"v"),"3.27.0","<") ? 1 : 0); } } exit(1);' \
  || { echo "twig/twig is not the remediated (>= 3.27.0) release"; exit 1; }
docker exec "$NAME" grep -q "__proto__" /var/www/html/js/vendor/js.cookie.js \
  || { echo "js-cookie prototype-pollution guard missing"; exit 1; }
# Front-end build metadata must NOT ship (it would only add phantom scanner findings).
docker exec "$NAME" sh -c '! test -e /var/www/html/yarn.lock && ! test -e /var/www/html/package.json' \
  || { echo "front-end build metadata leaked into the image"; exit 1; }

# php-fpm + nginx + supervisord all present.
for bin in php-fpm nginx supervisord; do
  docker exec "$NAME" sh -c "command -v $bin" >/dev/null || { echo "runtime tool '$bin' missing"; exit 1; }
done

rm -f "$out"
echo "smoke test passed (nonroot uid $user, read-only rootfs; nginx :8080 + php-fpm :9000 serve the phpMyAdmin login form)"
