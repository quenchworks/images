#!/usr/bin/env python3
# kgateway-envoy: the kgateway DATA PLANE, versioned in lockstep with the control plane
# because `envoyinit` is built from the same source tree and consumes the bootstrap that
# controller generates. So there is no separate upstream to poll: the update signal is
# scripts/check/kgateway.py, and the only thing worth checking here is that the two
# build.conf windows have not drifted apart -- a drift means a proxy fleet running an
# envoyinit from a different release than the controller that programs it.
import subprocess, sys
from _lib import report, github, current, BASE

def window(app):
    return subprocess.run(
        ["bash", "-c", f"source {BASE}/apps/{app}/build.conf; echo ${{VERSIONS[@]}} '|' ${{ENVOY_PKG[@]}}"],
        capture_output=True, text=True).stdout.split()

report("kgateway-envoy", github("kgateway-dev/kgateway"))

cp, dp = window("kgateway"), window("kgateway-envoy")
if cp != dp:
    print(f"  LOCKSTEP BROKEN:    apps/kgateway {cp}")
    print(f"                      apps/kgateway-envoy {dp}")
    sys.exit(1)
print(f"  lockstep:           matches apps/kgateway ({' '.join(current('kgateway'))})")
