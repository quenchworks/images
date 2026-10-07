#!/usr/bin/env bash
# Smoke test for a built OpenSCAP image. Usage: test.sh <image-ref> [version]
# Checks the version, validates a scap-security-guide datastream, and evaluates an OVAL
# definition (does /etc/os-release exist) that must come out true, with an XML report.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
WORK="$(mktemp -d "${TMPDIR:-$HOME}/oscap-smoke.XXXX")"
trap 'rm -rf "$WORK"' EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

ver="$(docker run --rm "$IMAGE" --version | sed -n 's/^OpenSCAP command line tool (oscap) \([0-9.]*\).*/\1/p')"
echo "oscap $ver"
[ -z "$WANT" ] || [ "$ver" = "$WANT" ] || { echo "expected version $WANT"; exit 1; }

# no /bin/sh or find in the image; bash comes with openscap
ds="$(docker run --rm --entrypoint /bin/bash "$IMAGE" -c 'f=(/usr/share/xml/scap/ssg/content/ssg-*-ds.xml); [ -e "${f[0]}" ] && echo "${f[0]}"')"
[ -n "$ds" ] || { echo "no scap-security-guide datastream in the image"; exit 1; }
docker run --rm "$IMAGE" ds sds-validate "$ds"
echo "datastream valid: $ds"

cat > "$WORK/def.xml" <<'XML'
<?xml version="1.0" encoding="UTF-8"?>
<oval_definitions xmlns="http://oval.mitre.org/XMLSchema/oval-definitions-5"
  xmlns:oval="http://oval.mitre.org/XMLSchema/oval-common-5"
  xmlns:unix="http://oval.mitre.org/XMLSchema/oval-definitions-5#unix">
  <generator><oval:schema_version>5.11.2</oval:schema_version><oval:timestamp>2026-10-07T00:00:00</oval:timestamp></generator>
  <definitions>
    <definition class="compliance" id="oval:io.quench:def:1" version="1">
      <metadata><title>os-release present</title><description>smoke</description></metadata>
      <criteria><criterion test_ref="oval:io.quench:tst:1" comment="os-release exists"/></criteria>
    </definition>
  </definitions>
  <tests>
    <unix:file_test id="oval:io.quench:tst:1" version="1" check="all" check_existence="all_exist" comment="file exists">
      <unix:object object_ref="oval:io.quench:obj:1"/>
    </unix:file_test>
  </tests>
  <objects>
    <unix:file_object id="oval:io.quench:obj:1" version="1">
      <unix:filepath>/etc/os-release</unix:filepath>
    </unix:file_object>
  </objects>
</oval_definitions>
XML
chmod 0755 "$WORK"; chmod 0644 "$WORK/def.xml"
out="$(docker run --rm -v "$WORK:/work:ro" "$IMAGE" oval eval /work/def.xml)"
echo "$out"
grep -q 'oval:io.quench:def:1: true' <<<"$out" || { echo "definition did not evaluate true"; exit 1; }
echo "smoke test passed"
