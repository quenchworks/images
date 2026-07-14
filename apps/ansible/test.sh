#!/usr/bin/env bash
# Smoke test for a built Ansible control-node image. Usage: test.sh <image-ref> [expected-community-version]
# A `--version` check alone is weak: it would pass even if the bundled collections or
# module machinery were broken. This test asserts the CLIs report a real version AND
# runs a genuine functional check — an ad-hoc `ansible -m ping localhost -c local`,
# which loads a module, executes it through the local connection, and must return
# SUCCESS/pong — as nonroot uid 1001.
set -euo pipefail

IMAGE="${1:?usage: test.sh <image-ref> [expected-community-version]}"
EXPECT_VER="${2:-}"

echo "checking the image runs as nonroot uid 1001"
USER_CFG="$(docker inspect --format '{{.Config.User}}' "$IMAGE")"
echo "  Config.User: $USER_CFG"
[ "$USER_CFG" = "1001" ] || { echo "FAIL: image user is not 1001"; exit 1; }

# `ansible --version` reports the ansible-CORE version (e.g. "core 2.21.1"), not the
# community meta-package version. Assert it prints a real "core X.Y.Z".
echo "ansible --version:"
VER_OUT="$(docker run --rm --entrypoint /usr/bin/ansible "$IMAGE" --version)"
echo "$VER_OUT" | sed 's/^/  /'
echo "$VER_OUT" | grep -qE 'core +[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "FAIL: ansible --version did not report a real core version"; exit 1; }

echo "ansible-playbook --version:"
PB_OUT="$(docker run --rm --entrypoint /usr/bin/ansible-playbook "$IMAGE" --version)"
echo "$PB_OUT" | sed 's/^/  /'
echo "$PB_OUT" | grep -qE 'core +[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "FAIL: ansible-playbook --version did not report a real core version"; exit 1; }

echo "ansible-galaxy --version:"
GX_OUT="$(docker run --rm --entrypoint /usr/bin/ansible-galaxy "$IMAGE" --version)"
echo "$GX_OUT" | sed 's/^/  /'
echo "$GX_OUT" | grep -qE '[0-9]+\.[0-9]+\.[0-9]+' \
  || { echo "FAIL: ansible-galaxy --version did not print a version"; exit 1; }

# Assert the COMMUNITY meta-package version if provided (via pip metadata inside the
# venv — this is the image tag, distinct from the core version above).
if [ -n "$EXPECT_VER" ]; then
  echo "verifying ansible community version == ${EXPECT_VER} (pip metadata)"
  COMM_VER="$(docker run --rm --entrypoint /opt/ansible/venv/bin/python "$IMAGE" \
    -c "import importlib.metadata as m; print(m.version('ansible'))")"
  echo "  community version: $COMM_VER"
  [ "$COMM_VER" = "$EXPECT_VER" ] \
    || { echo "FAIL: expected community version ${EXPECT_VER}, got ${COMM_VER}"; exit 1; }
fi

# FUNCTIONAL CHECK: run the ping module against localhost over the local connection.
# This exercises the real module loader + transport + Python module execution end to
# end and must report SUCCESS with a "pong". read-only rootfs + writable /tmp proves
# the control node works with no writable image layer.
echo "running: ansible -m ping localhost -c local (functional check)"
PING_OUT="$(docker run --rm --read-only --tmpfs /tmp \
  -e ANSIBLE_LOCAL_TEMP=/tmp/.ansible-tmp \
  -e ANSIBLE_REMOTE_TEMP=/tmp/.ansible-tmp \
  -e HOME=/tmp \
  --entrypoint /usr/bin/ansible "$IMAGE" \
  -m ping -c local localhost)"
echo "$PING_OUT" | sed 's/^/  /'
echo "$PING_OUT" | grep -q 'SUCCESS' \
  || { echo "FAIL: ansible ping did not report SUCCESS"; exit 1; }
echo "$PING_OUT" | grep -q '"ping": "pong"' \
  || { echo "FAIL: ansible ping did not return pong"; exit 1; }

echo "PASS: ansible control node smoke test green (CLIs report versions, ping localhost SUCCESS/pong, uid 1001)"
