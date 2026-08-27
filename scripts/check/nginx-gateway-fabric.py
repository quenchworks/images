#!/usr/bin/env python3
# nginx-gateway-fabric (control plane): GitHub releases (nginx/nginx-gateway-fabric),
# tag vX.Y.Z. Prereleases are flagged upstream, so github() already drops them.
#
# WINDOW IS 1 RELEASE (n defaults to len(VERSIONS) = 1). NGF 2.x is a version-locked
# pair: this control plane defaults the data-plane image TAG to its own ldflag version,
# and every release pins its own nginx / njs / nginx-agent trio. See
# apps/nginx-gateway-fabric/build.conf.
#
# Coupling the checker cannot see, and that a bump MUST re-check by hand:
#   1. apps/nginx-gateway-fabric-nginx must move to the same version, with the
#      nginx / njs / agent pins from the new release's build/Dockerfile.nginx.
#   2. The release's vendored sigs.k8s.io/gateway-api version (go.mod) decides which
#      Gateway API bundle it accepts. A MAJOR mismatch against the installed
#      gateway-api-crds chart leaves every GatewayClass Accepted=False; a minor
#      mismatch is accepted "best effort" (internal/controller/state/graph/
#      gatewayclass.go validateCRDVersions). v2.6.7 vendors gateway-api v1.5.1.
#   3. The CRDs in charts/quench/nginx-gateway-fabric/crds/ are release artifacts and
#      have to be re-vendored from the new tag.
from _lib import github, report
report("nginx-gateway-fabric", github("nginx/nginx-gateway-fabric"))
