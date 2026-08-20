#!/usr/bin/env python3
# contour: GitHub releases, tag vX.Y.Z.
#
# The window is 1 on purpose (see apps/contour/build.conf): a Contour release pairs with
# an EXACT Envoy version, and we can only ship a Contour line whose Envoy minor exists in
# apps/envoy/build.conf. So the check does not stop at "is there a newer tag" -- it also
# reads the Envoy image the candidate tag actually deploys and says whether we ship that
# minor. A newer Contour we cannot pair with an Envoy is not an available update.
import re, subprocess, sys, urllib.request
from _lib import github, report, current, vkey, BASE

def envoy_pin(tag):
    """Envoy version the tag's own Envoy DaemonSet deploys (the real authority; the
    compatibility-matrix doc and versions.yaml go stale on release branches)."""
    url = f"https://raw.githubusercontent.com/projectcontour/contour/v{tag}/examples/contour/03-envoy.yaml"
    try:
        y = urllib.request.urlopen(url, timeout=30).read().decode("utf-8", "replace")
    except Exception as e:                                    # noqa: BLE001
        return None, f"could not fetch 03-envoy.yaml ({e})"
    m = re.search(r"image:\s*docker\.io/envoyproxy/envoy:(?:distroless-)?v([0-9.]+)", y)
    return (m.group(1), None) if m else (None, "no envoyproxy/envoy image in 03-envoy.yaml")

def our_envoy_lines():
    out = subprocess.run(
        ["bash", "-c", f"source {BASE}/apps/envoy/build.conf; echo ${{VERSIONS[@]}}"],
        capture_output=True, text=True).stdout.split()
    return {".".join(v.split(".")[:2]): v for v in out}

behind = report("contour", github("projectcontour/contour"))

cand = (behind or current("contour"))[-1]
pin, err = envoy_pin(cand)
lines = our_envoy_lines()
if err:
    print(f"  envoy pairing:      contour {cand} -> UNKNOWN ({err})")
    sys.exit(0)
line = ".".join(pin.split(".")[:2])
ours = lines.get(line)
if not ours:
    print(f"  envoy pairing:      contour {cand} wants envoy {pin}; we ship "
          f"{sorted(lines, key=vkey)} -- NO {line} LINE, cannot pair (do not bump)")
elif vkey(ours) < vkey(pin):
    print(f"  envoy pairing:      contour {cand} wants envoy {pin}; we ship {ours} "
          f"(same {line} line, ours is older) -- pairable, bump envoy when Wolfi has {pin}")
else:
    print(f"  envoy pairing:      contour {cand} wants envoy {pin}; we ship {ours} -- pairable")
