#!/usr/bin/env bash
# Smoke test for a built quench-jre image. Usage: test.sh <image-ref> <major>
# where <major> is the expected OpenJDK major version, e.g. 21.
#
# This is a LANGUAGE RUNTIME image: the `java` launcher IS the entrypoint, there
# is no long-running service to ping. So we exercise the JRE directly under a
# READ-ONLY rootfs and confirm the nonroot uid. Crucially this is RUNTIME-ONLY:
# we assert that the compiler (`javac`) is ABSENT, which is what distinguishes
# this image from quench-jdk. `java -version` writes to stderr (normal for JVM).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <major>}"
MAJOR="${2:?usage: test.sh <image-ref> <major>}"   # e.g. 21

echo "== java -version reports major $MAJOR (default entrypoint) =="
# `java -version` prints to stderr; capture both. Output looks like:
#   openjdk version "21.0.11" 2026-04-21
ver="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -version 2>&1)"
echo "$ver"
case "$ver" in
  *"openjdk version \"$MAJOR."*|*"openjdk version \"$MAJOR\""*) : ;;
  *) echo "expected 'openjdk version \"$MAJOR.*'\"', got '$ver'"; exit 1 ;;
esac

echo "== javac is ABSENT (this is a runtime-only JRE, not a JDK) =="
# A JRE must NOT ship the compiler. Invoking javac should fail because the
# binary does not exist in the image.
if docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/javac \
     "$IMAGE" -version >/dev/null 2>&1; then
  echo "FAIL: javac ran -- this image is not runtime-only"; exit 1
fi
echo "ok: /usr/bin/javac is not present"

echo "== jshell is ABSENT (no compiler/REPL in the JRE) =="
if docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/jshell \
     "$IMAGE" --help >/dev/null 2>&1; then
  echo "FAIL: jshell ran -- this image is not runtime-only"; exit 1
fi
echo "ok: /usr/bin/jshell is not present"

echo "== java.home resolves to the matching major =="
jhome="$(docker run --rm --read-only --tmpfs /tmp \
  --entrypoint /usr/bin/java "$IMAGE" -XshowSettings:properties -version 2>&1 \
  | sed -n 's/.*java\.home = //p')"
echo "java.home = $jhome"
case "$jhome" in
  *"java-$MAJOR-openjdk"*) : ;;
  *) echo "expected java.home to contain java-$MAJOR-openjdk, got '$jhome'"; exit 1 ;;
esac

echo "== the runtime starts and lists modules on a read-only rootfs =="
# A JRE has no compiler, so we cannot compile-and-run source. Instead prove the
# launcher + module system come up cleanly under a read-only rootfs by listing
# the resolved modules and confirming the base module is present. This exercises
# the JVM end to end without needing javac or a writable workdir.
mods="$(docker run --rm --read-only --tmpfs /tmp \
  --entrypoint /usr/bin/java "$IMAGE" --list-modules 2>&1)"
case "$mods" in
  *"java.base@$MAJOR."*) : ;;
  *) echo "FAIL: expected java.base@$MAJOR.* in --list-modules, got:"; echo "$mods" | head; exit 1 ;;
esac
# And confirm the compiler module is NOT bundled (belt-and-braces runtime check).
case "$mods" in
  *"jdk.compiler"*) echo "FAIL: jdk.compiler module present -- not a pure JRE"; exit 1 ;;
  *) : ;;
esac
echo "ok: java.base@$MAJOR present, jdk.compiler absent"

echo "== runs as configured nonroot uid 1001 =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== entrypoint is the java launcher =="
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
case "$ep" in
  *"/usr/bin/java"*) : ;;
  *) echo "unexpected entrypoint: $ep"; exit 1 ;;
esac

echo "smoke test passed (OpenJDK JRE $MAJOR, runtime-only, nonroot uid: 1001, read-only rootfs)"
