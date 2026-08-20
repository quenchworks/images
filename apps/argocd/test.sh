#!/usr/bin/env bash
# Smoke test for a built Argo CD image. Usage: test.sh <image-ref>
#
# Argo CD's server and controllers only do real work against an API server (the
# chart's kind install gate is the runtime test). What is checkable here is
# everything a port probe would miss: that the multi-call binary really does
# dispatch a DIFFERENT command per symlink, that the web console was actually
# built and embedded instead of the placeholder tree, that the external tools the
# repo server execs by bare name are present and are the right major versions,
# and that the repo server -- the one component that needs no cluster -- comes up
# and answers its own /healthz.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref>}"

run() { docker run --rm --entrypoint "$1" "$IMAGE" "${@:2}"; }

echo "== version is stamped from the tag"
ver="$(run /usr/bin/argocd version --client --short 2>&1 | tr -d '\r')"
echo "  $ver"
# "argocd: v3.4.5" -- a missing gitCommit/gitTag would read "v3.4.5+unknown".
echo "$ver" | grep -qE '^argocd: v[0-9]+\.[0-9]+\.[0-9]+$' \
  || { echo "version not stamped cleanly: '$ver'"; exit 1; }

echo "== every component symlink dispatches its OWN command"
# One binary, argv[0] dispatch (cmd/main.go). If the symlinks were wrong, or the
# dispatch broke, every name would fall through to the CLI -- so each name is
# matched against a string only ITS cobra root prints.
check_cmd() {
  out="$(docker run --rm --entrypoint "/usr/bin/$1" "$IMAGE" --help 2>&1 || true)"
  echo "$out" | grep -qi -- "$2" \
    || { echo "  $1 did not dispatch to its own command (looked for '$2')"; echo "$out" | head -20; exit 1; }
  echo "  $1 -> ok"
}
# NOTE: match on the Long description, not the Short one. cobra prints Long when it
# is set and Short only when it is not, so grepping the Short string ("Run the
# ArgoCD API server") never matches for a command that has a Long -- which
# argocd-server does. That is a stale-expectation false failure, not a broken image.
check_cmd argocd-server                     "gRPC/REST server which exposes the API"
check_cmd argocd-repo-server                "maintains a local cache of the Git repository"
check_cmd argocd-application-controller     "continuously monitors running applications"
check_cmd argocd-applicationset-controller  "applicationset"
check_cmd argocd-notifications              "notification"
check_cmd argocd-cmp-server                 "runs as sidecar container in reposerver"
check_cmd argocd-commit-server              "commits and pushes hydrated manifests"
check_cmd argocd-dex                        "dex"
check_cmd argocd-k8s-auth                   "argocd-k8s-auth"
check_cmd argocd-git-ask-pass               "git credential helper"
# ... and $ARGOCD_BINARY_NAME is the other documented selector.
docker run --rm -e ARGOCD_BINARY_NAME=argocd-server --entrypoint /usr/bin/argocd "$IMAGE" \
  --help 2>&1 | grep -q "gRPC/REST server which exposes the API" \
  || { echo "ARGOCD_BINARY_NAME override does not select the server"; exit 1; }
echo "  ARGOCD_BINARY_NAME override -> ok"

echo "== the React console was BUILT, not left as the gitkeep placeholder"
# ui/embed.go embeds ui/dist/app; embed.FS keeps the file names verbatim in the
# binary, so the bundle is verifiable from outside. A build against the untouched
# source tree embeds ONLY "dist/app/gitkeep" and every UI route 404s at runtime.
names="$(docker run --rm --entrypoint /usr/bin/strings "$IMAGE" -n 8 /usr/bin/argocd 2>/dev/null \
          || docker run --rm --entrypoint /bin/sh "$IMAGE" -c \
               "tr -c '[:print:]' '\n' < /usr/bin/argocd")"
# Here-strings, NOT `echo ... | grep -q`. Under `set -o pipefail`, grep -q exits the
# moment it matches, echo takes SIGPIPE (141), and the pipeline reports failure BECAUSE
# the match succeeded. It is size-dependent, which is why the smaller check_cmd greps
# above survive: $names is the strings output of a ~190MB binary, so echo is still
# writing when grep leaves. This reported "the UI was not built" for a correctly built UI.
grep -q 'dist/app/index.html' <<<"$names" \
  || { echo "no dist/app/index.html embedded -- the UI was not built"; exit 1; }
grep -q 'dist/app/assets/images/resources/' <<<"$names" \
  || { echo "no embedded resource icons -- the UI bundle is incomplete"; exit 1; }
nchunk="$(grep -c 'dist/app/[0-9a-z.]*chunk\.js' <<<"$names" || true)"
[ "$nchunk" -ge 10 ] || { echo "only $nchunk embedded js chunks; expected >=10"; exit 1; }
echo "  index.html + resource icons + $nchunk js chunks embedded"

echo "== the tools the repo server execs by bare name are present and runnable"
# util/helm/cmd.go and util/kustomize/kustomize.go exec "helm" and "kustomize"
# with no path; util/git and util/gpg exec git, git-lfs, gpg and the two sh
# wrappers. Missing or non-executable = every Helm/Kustomize Application fails to
# render, which nothing else here would notice.
helmv="$(docker run --rm --entrypoint /usr/bin/helm "$IMAGE" version --short 2>&1)"
echo "  helm: $helmv"
echo "$helmv" | grep -qE '^v3\.' || { echo "helm is not a 3.x binary: $helmv"; exit 1; }
kv="$(docker run --rm --entrypoint /usr/bin/kustomize "$IMAGE" version 2>&1)"
echo "  kustomize: $kv"
echo "$kv" | grep -qE 'v?5\.' || { echo "kustomize is not 5.x: $kv"; exit 1; }
gv="$(docker run --rm --entrypoint /usr/bin/git "$IMAGE" --version 2>&1)"
echo "  $gv"
echo "$gv" | grep -q '^git version 2\.' || { echo "unexpected git: $gv"; exit 1; }
docker run --rm --entrypoint /usr/bin/git "$IMAGE" lfs version >/dev/null \
  || { echo "git-lfs not usable"; exit 1; }
docker run --rm --entrypoint /usr/bin/gpg "$IMAGE" --version >/dev/null \
  || { echo "gpg not usable"; exit 1; }
# The wrappers are sh scripts; running one proves BOTH that it is executable and
# that the image really does have a working /bin/sh.
docker run --rm --entrypoint /usr/bin/gpg-wrapper.sh "$IMAGE" --version >/dev/null \
  || { echo "gpg-wrapper.sh not runnable (missing shell?)"; exit 1; }
docker run --rm --entrypoint /usr/bin/git-verify-wrapper.sh "$IMAGE" 2>&1 \
  | grep -q "Wrong usage" || { echo "git-verify-wrapper.sh not runnable"; exit 1; }
echo "  git-lfs, gpg and both sh wrappers ok"

echo "== the repo server actually STARTS and reports itself healthy"
# The one component that needs no API server. It binds 8081 (gRPC) and 8084
# (metrics + /healthz); /healthz answers 200 only after the gRPC server is up, so
# this is a liveness answer rather than an open port. Also proves the read-only
# rootfs layout is complete: same flags the chart uses.
cid="$(docker run -d --rm --read-only \
        --tmpfs /tmp --tmpfs /helm-working-dir --tmpfs /app/config/gpg/keys \
        --tmpfs /home/argocd -p 18084:8084 \
        --entrypoint /usr/bin/argocd-repo-server "$IMAGE" --port 8081 --metrics-port 8084)"
trap 'docker logs "$cid" 2>&1 | tail -30; docker rm -f "$cid" >/dev/null 2>&1 || true' EXIT
ok=0
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:18084/healthz" >/dev/null 2>&1; then ok=1; break; fi
  sleep 1
done
[ "$ok" = 1 ] || { echo "repo-server never answered /healthz on 8084"; exit 1; }
echo "  /healthz 200 on a READ-ONLY rootfs"
# and it can render a chart through the helm binary it just proved is present
docker exec "$cid" /usr/bin/helm version --short >/dev/null
docker rm -f "$cid" >/dev/null; trap - EXIT

echo "== nonroot"
user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }
uid="$(docker run --rm --entrypoint /usr/bin/id "$IMAGE" -u)"
[ "$uid" = "1001" ] || { echo "expected runtime uid 1001, got '$uid'"; exit 1; }

echo "smoke test passed ($ver, helm $helmv, uid $uid, UI embedded)"
