#!/usr/bin/env bash
# Smoke test for a built openldap image. Usage: test.sh <image-ref> [version]
# This is a REAL directory test, not a version banner: it starts slapd on 1389 with
# the image's baked default config, then exercises bind + add + search + whoami
# through the bundled client tools.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="openldap-smoke-$$"
BASE="dc=my-domain,dc=com"           # the suffix in the image's default slapd.conf
BINDDN="cn=Manager,${BASE}"
BINDPW="secret"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "starting slapd ..."
docker run -d --name "$NAME" "$IMAGE" >/dev/null

ready=0
for i in $(seq 1 30); do
  if docker exec "$NAME" ldapsearch -x -H ldap://127.0.0.1:1389 -b "" -s base -LLL >/dev/null 2>&1; then
    ready=1; break
  fi
  sleep 1
done
[ "$ready" = 1 ] || { echo "slapd never answered on 1389"; docker logs "$NAME" | tail -30; exit 1; }
echo "slapd is answering on 1389"

echo "ldapwhoami (authenticated bind as $BINDDN)"
who="$(docker exec "$NAME" ldapwhoami -x -H ldap://127.0.0.1:1389 -D "$BINDDN" -w "$BINDPW")"
echo "  -> $who"
echo "$who" | grep -qi "^dn:cn=Manager,${BASE}$" || { echo "unexpected whoami: $who"; exit 1; }

echo "ldapadd the base entry"
docker exec -i "$NAME" ldapadd -x -H ldap://127.0.0.1:1389 -D "$BINDDN" -w "$BINDPW" <<LDIF >/dev/null
dn: ${BASE}
objectClass: dcObject
objectClass: organization
dc: my-domain
o: QuenchWorks Smoke Test

dn: cn=smoke,${BASE}
objectClass: organizationalRole
cn: smoke
LDIF

echo "ldapsearch the seeded subtree"
out="$(docker exec "$NAME" ldapsearch -x -H ldap://127.0.0.1:1389 -b "$BASE" -LLL "(cn=smoke)" dn)"
echo "$out"
echo "$out" | grep -qi "cn=smoke,${BASE}" || { echo "search did not return the seeded entry"; exit 1; }

echo "slappasswd (admin tooling present)"
docker exec "$NAME" slappasswd -s hunter2 | grep -q '^{SSHA}' || { echo "slappasswd broken"; exit 1; }

ver="$(docker exec "$NAME" slapd -VV 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)"
[ -n "$ver" ] || { echo "slapd did not report a version"; exit 1; }
if [ -n "$WANT" ] && [ "$ver" != "$WANT" ]; then
  echo "version mismatch: got $ver, want $WANT"; exit 1
fi

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

echo "smoke test passed (openldap $ver, nonroot $user, bind+add+search on 1389)"
