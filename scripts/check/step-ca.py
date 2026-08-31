#!/usr/bin/env python3
# step-ca: TWO upstreams, because the image ships two binaries (see build.conf):
#   step-ca  <- smallstep/certificates  (VERSIONS / SHA256)
#   step CLI <- smallstep/cli           (the STEP / STEP_SHA maps)
# They have separate release trains that only loosely track each other (today:
# certificates 0.30.2, cli 0.30.6), so checking only the server would hide a CVE
# fix in the bundled CLI. Both are GitHub releases with vX.Y.Z tags, newest-only.
import subprocess

from _lib import BASE, github, report, vkey

report("step-ca", github("smallstep/certificates"), n=1)

# The CLI pin lives in build.conf's STEP assoc array, not VERSIONS, so current()
# cannot read it.
step = subprocess.run(
    ["bash", "-c", f"source {BASE}/apps/step-ca/build.conf; echo ${{STEP[@]}}"],
    capture_output=True, text=True).stdout.split()
cands = sorted((t for t in github("smallstep/cli") if t[:1].isdigit()), key=vkey, reverse=True)
have = sorted(step, key=vkey)[-1] if step else None
newest = cands[0] if cands else None
if not newest:
    status = "BROKEN CHECK -- smallstep/cli returned NO candidates"
elif not have:
    status = "BROKEN CHECK -- no STEP pin found in build.conf"
elif vkey(newest) > vkey(have):
    status = f"UPDATE -> ['{newest}']"
else:
    status = "ok"
print(f"{'step-ca (step cli)':20s} have={(have or '?'):>14s}  latest1={[newest] if newest else []}  {status}")
