#!/usr/bin/env bash
# Smoke test for a built quench-gradle image. Usage: test.sh <image-ref> <gradlemajor> <jdkmajor>
# e.g. test.sh quench-gradle:9-amd64 9 21
#
# This is a BUILD-TOOL image: the `gradle` launcher (a /bin/sh wrapper that
# drives the JDK) IS the entrypoint, there is no long-running service. We
# exercise gradle + the bundled JDK directly under a READ-ONLY rootfs and
# confirm the nonroot uid.
#
# NOTE: on a fresh GRADLE_USER_HOME (a clean /tmp each run) Gradle prints a
# one-time "Welcome" banner to stderr; this is cosmetic and `gradle --version`
# still exits 0 with the real report on STDOUT. Every assertion reads STDOUT.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <gradlemajor> <jdkmajor>}"
GRADLE="${2:?usage: test.sh <image-ref> <gradlemajor> <jdkmajor>}"   # e.g. 9
JDK="${3:?usage: test.sh <image-ref> <gradlemajor> <jdkmajor>}"      # e.g. 21

echo "== gradle --version reports Gradle $GRADLE + JVM $JDK (default entrypoint, STDOUT only) =="
out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version 2>/dev/null)"
echo "$out"
case "$out" in
  *"Gradle $GRADLE."*) : ;;
  *) echo "expected 'Gradle $GRADLE.*', got STDOUT above"; exit 1 ;;
esac
case "$out" in
  *"Launcher JVM:  $JDK."*) : ;;
  *) echo "expected 'Launcher JVM:  $JDK.*' in the report"; exit 1 ;;
esac
case "$out" in
  *"/usr/lib/jvm/java-$JDK-openjdk"*) : ;;
  *) echo "expected Daemon JVM at /usr/lib/jvm/java-$JDK-openjdk"; exit 1 ;;
esac

echo "== gradle exits 0 on the read-only rootfs (GRADLE_USER_HOME on /tmp) =="
docker run --rm --read-only --tmpfs /tmp "$IMAGE" --version >/dev/null 2>&1 \
  || { echo "gradle --version returned non-zero"; exit 1; }

echo "== the bundled JDK is the expected major (java -version, STDERR is normal) =="
jver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/java "$IMAGE" -version 2>&1)"
echo "$jver"
case "$jver" in
  *"openjdk version \"$JDK."*|*"openjdk version \"$JDK\""*) : ;;
  *) echo "expected 'openjdk version \"$JDK.*'\"', got '$jver'"; exit 1 ;;
esac

echo "== JAVA_HOME points at the JDK install =="
jh="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/printenv "$IMAGE" JAVA_HOME 2>/dev/null || true)"
echo "JAVA_HOME=$jh"
[ "$jh" = "/usr/lib/jvm/java-$JDK-openjdk" ] \
  || { echo "expected JAVA_HOME /usr/lib/jvm/java-$JDK-openjdk, got '$jh'"; exit 1; }

echo "== runs as configured nonroot uid 1001 =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== entrypoint is the gradle launcher =="
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
case "$ep" in
  *"/usr/bin/gradle"*) : ;;
  *) echo "unexpected entrypoint: $ep"; exit 1 ;;
esac

echo "smoke test passed (Gradle $GRADLE on OpenJDK $JDK, nonroot uid: 1001, read-only rootfs)"
