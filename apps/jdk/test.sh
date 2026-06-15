#!/usr/bin/env bash
# Smoke test for a built quench-jdk image. Usage: test.sh <image-ref> <major>
# where <major> is the expected OpenJDK major version, e.g. 21.
#
# This is a LANGUAGE/JDK image: the `java` launcher IS the entrypoint, there is
# no long-running service to ping. So we exercise the JDK directly under a
# READ-ONLY rootfs and confirm the nonroot uid. `java -version` writes to
# stderr (this is normal for the JVM). We compile + run real code shell-lessly
# via `jshell` fed from stdin -- this proves the compiler is wired up without
# needing a writable class-output dir or a shell in the image.
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

echo "== javac -version reports major $MAJOR (compiler present) =="
# `javac -version` may print to stdout or stderr depending on the JDK; merge.
jcver="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/bin/javac \
  "$IMAGE" -version 2>&1)"
echo "$jcver"
case "$jcver" in
  "javac $MAJOR."*|"javac $MAJOR "*) : ;;
  *) echo "expected 'javac $MAJOR.*', got '$jcver'"; exit 1 ;;
esac

echo "== JAVA_HOME points at the JDK and resolves to the same major =="
jhome="$(docker run --rm --read-only --tmpfs /tmp \
  --entrypoint /usr/bin/java "$IMAGE" -XshowSettings:properties -version 2>&1 \
  | sed -n 's/.*java\.home = //p')"
echo "java.home = $jhome"
[ "$jhome" = "/usr/lib/jvm/java-$MAJOR-openjdk" ] \
  || { echo "expected java.home /usr/lib/jvm/java-$MAJOR-openjdk, got '$jhome'"; exit 1; }

echo "== compile + run real code via jshell on a read-only rootfs =="
# jshell reads snippets from stdin (no shell, no writable workdir needed).
# Point the java.util.prefs roots at the writable tmpfs so jshell does not warn
# about an unwritable prefs dir, and use --feedback silent for clean output.
out="$(printf 'System.out.print("quench-jdk-ok");\n/exit\n' | \
  docker run --rm --interactive --read-only --tmpfs /tmp \
    --entrypoint /usr/bin/jshell "$IMAGE" \
    -J-Djava.util.prefs.userRoot=/tmp -J-Djava.util.prefs.systemRoot=/tmp \
    --feedback silent - 2>/dev/null)"
echo "jshell output: [$out]"
[ "$out" = "quench-jdk-ok" ] || { echo "jshell compile+run failed: got '$out'"; exit 1; }

echo "== runs as configured nonroot uid 1001 =="
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected configured user 1001, got '$user'"; exit 1; }

echo "== entrypoint is the java launcher =="
ep="$(docker inspect "$IMAGE" --format '{{json .Config.Entrypoint}}')"
case "$ep" in
  *"/usr/bin/java"*) : ;;
  *) echo "unexpected entrypoint: $ep"; exit 1 ;;
esac

echo "smoke test passed (OpenJDK $MAJOR, nonroot uid: 1001, read-only rootfs)"
