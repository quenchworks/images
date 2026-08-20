#!/usr/bin/env python3
# envoy-gateway: GitHub releases (envoyproxy/gateway), tag vX.Y.Z. Release candidates
# are flagged prerelease upstream, so github() already drops them.
#
# WINDOW IS 2 MINOR LINES, NOT 3 (see apps/envoy-gateway/build.conf): 1.7.x links
# github.com/docker/docker v28.5.2+incompatible, whose HIGH has no Go module fix, so it
# cannot pass the gate. n defaults to len(VERSIONS) = 2, which is what keeps a fresh
# 1.7.x patch from being proposed as an update.
#
# Coupling to watch on every bump, because the checker cannot see it: each release pins
# a DIFFERENT default data-plane image (api/v1alpha1/shared_types.go
# DefaultEnvoyProxyImage) and a different sigs.k8s.io/gateway-api. A new minor may want
# an envoy line Wolfi has not packaged yet -- 1.9.0 already wants envoy 1.39 while
# apps/envoy tops out at the envoy-1.38 apk. `envoy-gateway version` in the image
# reports both, and test.sh prints them.
from _lib import github, report
report("envoy-gateway", github("envoyproxy/gateway"))
