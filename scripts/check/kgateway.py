#!/usr/bin/env python3
# kgateway: GitHub releases (kgateway-dev/kgateway), tag vX.Y.Z. Release candidates and
# betas are flagged prerelease upstream, so github() already drops them.
#
# WINDOW IS 2 MINOR LINES, NOT 3 (see apps/kgateway/build.conf): 2.2.x pins its amd64
# data plane to quay.io/solo-io/envoy-gloo, a vendor fork of Envoy we neither ship nor
# can rebuild from Wolfi. n defaults to len(VERSIONS) = 2, which keeps a fresh 2.2.x
# patch from being proposed as an update.
#
# The version check alone is not enough for this app, so it also reads the ENVOY pin the
# candidate tag actually builds against (Makefile `export ENVOY_IMAGE`) and says whether
# apps/envoy ships that MINOR. kgateway's xDS is generated against a specific Envoy
# minor; a kgateway release whose Envoy minor Wolfi has not packaged cannot be shipped
# here, which is exactly the wall 1.39-era Envoy Gateway hit.
#
# apps/kgateway-envoy is in lockstep with this app -- scripts/check/kgateway-envoy.py
# asserts that rather than repeating the release lookup.
import re, subprocess, sys, urllib.request
from _lib import github, report, current, vkey, BASE

def envoy_pin(tag):
    """The Envoy image the tag's own Makefile builds its data plane from."""
    url = f"https://raw.githubusercontent.com/kgateway-dev/kgateway/v{tag}/Makefile"
    try:
        mk = urllib.request.urlopen(url, timeout=30).read().decode("utf-8", "replace")
    except Exception as e:                                    # noqa: BLE001
        return None, f"could not fetch the Makefile ({e})"
    # 2.3+ has one pin; 2.2.x had ENVOY_IMAGE_AMD64 (envoy-gloo) + ENVOY_IMAGE_ARM64.
    m = re.search(r"^export ENVOY_IMAGE \??=\s*\S*envoy:v?([0-9.]+)", mk, re.M)
    if m:
        return m.group(1), None
    if re.search(r"^export ENVOY_IMAGE_AMD64\s*=\s*\S*envoy-gloo", mk, re.M):
        return None, "pins the solo-io envoy-gloo fork on amd64 -- not shippable here"
    return None, "no ENVOY_IMAGE in the Makefile"

def our_envoy_lines():
    out = subprocess.run(
        ["bash", "-c", f"source {BASE}/apps/envoy/build.conf; echo ${{VERSIONS[@]}}"],
        capture_output=True, text=True).stdout.split()
    return {".".join(v.split(".")[:2]): v for v in out}

behind = report("kgateway", github("kgateway-dev/kgateway"))

cand = (behind or current("kgateway"))[-1]
pin, err = envoy_pin(cand)
if err:
    print(f"  envoy pairing:      kgateway {cand} -> UNKNOWN/BLOCKED ({err})")
    sys.exit(0)
lines = our_envoy_lines()
line = ".".join(pin.split(".")[:2])
ours = lines.get(line)
if not ours:
    print(f"  envoy pairing:      kgateway {cand} wants envoy {pin}; we ship "
          f"{sorted(lines, key=vkey)} -- NO {line} LINE, cannot pair (do not bump)")
elif vkey(ours) < vkey(pin):
    print(f"  envoy pairing:      kgateway {cand} wants envoy {pin}; we ship {ours} "
          f"(same {line} line, ours is older) -- pairable, bump envoy when Wolfi has {pin}")
else:
    print(f"  envoy pairing:      kgateway {cand} wants envoy {pin}; we ship {ours} -- pairable")
