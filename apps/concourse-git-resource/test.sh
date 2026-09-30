#!/usr/bin/env bash
# Smoke test for the Concourse git resource image. Usage: test.sh <image-ref> [expected-version]
#
# Runs the real resource protocol the way a Concourse worker does: JSON on stdin to
# /opt/resource/check and /opt/resource/in, against a git repository made in the
# container (file:// is allowed by the image's /etc/gitconfig, as upstream).
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-version]}"
fail() { echo "FAIL: $*"; exit 1; }

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || fail "expected user 1001, got '$user'"

out="$(docker run --rm --entrypoint /bin/bash "$IMAGE" -c '
  set -euo pipefail
  git init -q -b main /tmp/repo
  cd /tmp/repo && echo one > f && git add f && git commit -q -m one
  first="$(git rev-parse HEAD)"
  echo two > f && git commit -q -am two
  head="$(git rev-parse HEAD)"
  src="{\"uri\":\"file:///tmp/repo\",\"branch\":\"main\"}"
  checked="$(echo "{\"source\":$src}" | /opt/resource/check)"
  echo "check: $checked" >&2
  jq -e --arg h "$head" "any(.[]; .ref == \$h)" <<<"$checked" >/dev/null
  got="$(echo "{\"source\":$src,\"version\":{\"ref\":\"$first\"}}" | /opt/resource/in /tmp/out)"
  echo "in: $got" >&2
  jq -e --arg f "$first" ".version.ref == \$f" <<<"$got" >/dev/null
  [ "$(cat /tmp/out/f)" = one ]
  [ "$(git -C /tmp/out rev-parse HEAD)" = "$first" ]
  git lfs version >/dev/null && git-crypt --version >/dev/null && gpg --version >/dev/null && ssh -V 2>/dev/null
  proxytunnel --version 2>&1 | head -1
  echo resource-ok
')" || fail "the resource protocol run failed"
grep -q '^resource-ok$' <<<"$out" || fail "the resource protocol run did not finish: $out"
echo "smoke test passed (check found HEAD, in fetched a pinned ref, uid $user, $(head -1 <<<"$out"))"
