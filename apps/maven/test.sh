#!/usr/bin/env bash
# Smoke test for a built quench-maven image. Usage: test.sh <image-ref> <jdkmajor>
# where <jdkmajor> is the expected underlying OpenJDK major, e.g. 21.
#
# This is a BUILD-TOOL image: the `mvn` launcher (a /bin/sh wrapper that drives
# the JDK) IS the entrypoint, there is no long-running service. We exercise mvn
# + the bundled JDK directly under a READ-ONLY rootfs and confirm the nonroot
# uid.
#
# NOTE on jansi: when /tmp is mounted `noexec` (the Docker `--tmpfs /tmp`
# default), Maven's optional jansi colour library logs a harmless stderr warning
# because it cannot exec its extracted native .so. Maven still runs and exits 0,
# and STDOUT stays clean -- so every assertion below reads mvn's STDOUT only,
# exactly like the JDK image treats the JVM's stderr banner. A real k8s
# read-only-rootfs deployment with an exec-capable emptyDir at /tmp never sees it.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> <jdkmajor>}"
JDK="${2:?usage: test.sh <image-ref> <jdkmajor>}"   # e.g. 21

echo "== mvn -version reports Maven 3.9 + JDK $JDK (default entrypoint, STDOUT only) =="
# Read STDOUT only (2>/dev/null) so the benign jansi noexec warning is ignored.
out="$(docker run --rm --read-only --tmpfs /tmp "$IMAGE" -version 2>/dev/null)"
echo "$out"
case "$out" in
  *"Apache Maven 3.9."*) : ;;
  *) echo "expected 'Apache Maven 3.9.*', got '$out'"; exit 1 ;;
esac
case "$out" in
  *"Java version: $JDK."*) : ;;
  *) echo "expected 'Java version: $JDK.*', got '$out'"; exit 1 ;;
esac

echo "== mvn exits 0 even with the read-only rootfs + noexec /tmp =="
docker run --rm --read-only --tmpfs /tmp "$IMAGE" -version >/dev/null 2>&1 \
  || { echo "mvn -version returned non-zero"; exit 1; }

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

echo "== entrypoint is the mvn launcher =="
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
case "$ep" in
  *"/usr/bin/mvn"*) : ;;
  *) echo "unexpected entrypoint: $ep"; exit 1 ;;
esac

echo "smoke test passed (Maven 3.9 on OpenJDK $JDK, nonroot uid: 1001, read-only rootfs)"
