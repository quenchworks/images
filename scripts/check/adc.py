#!/usr/bin/env python3
# adc: the apisix-ingress-controller SIDECAR (api7/adc). Its version is NOT ours
# to pick -- it is whatever the controller release we ship pins, so checking
# api7/adc's tag list would report "updates" we must not take. The controller's
# Makefile is the real upstream source for this app, so that is what we read:
#
#   apache/apisix-ingress-controller @ <our controller version> -> `ADC_VERSION ?= X.Y.Z`
#
# api7/adc's own newest tag is printed as INFORMATION only. Taking it means
# bumping the controller too (see apps/adc/build.conf: the server wire schema
# moved between 0.27.1 and 0.29.0, and 0.29.0 wants a pnpm newer than Wolfi's).
import re, subprocess, sys, urllib.request
from _lib import BASE, current, github_tags, report, vkey

CONTROLLER = "apisix-ingress-controller"

# The controller version we actually ship; build.conf's PAIRED_CONTROLLER must
# agree with it, or the two apps have drifted in this repo.
ctl = current(CONTROLLER)
ctl_ver = ctl[-1] if ctl else None
if not ctl_ver:
    sys.exit(f"adc: cannot read apps/{CONTROLLER}/build.conf VERSIONS -- broken check")

# apps/adc/build.conf records which controller release this sidecar is paired
# with. If it disagrees with the controller we actually ship, the two apps have
# drifted inside this repo -- which is the failure this whole checker exists to
# prevent, so it is fatal, not a warning.
paired = subprocess.run(
    ["bash", "-c", f"source {BASE}/apps/adc/build.conf; echo ${{PAIRED_CONTROLLER:-}}"],
    capture_output=True, text=True).stdout.strip()
if paired != ctl_ver:
    sys.exit(f"adc: build.conf PAIRED_CONTROLLER={paired!r} but we ship "
             f"{CONTROLLER} {ctl_ver} -- the sidecar/controller pair has drifted")

url = f"https://raw.githubusercontent.com/apache/{CONTROLLER}/{ctl_ver}/Makefile"
try:
    mk = urllib.request.urlopen(url, timeout=30).read().decode("utf-8", "replace")
except Exception as e:
    sys.exit(f"adc: cannot fetch {url}: {e} -- broken check")

m = re.search(r'^ADC_VERSION\s*\?=\s*([0-9][^\s#]*)', mk, re.M)
if not m:
    sys.exit(f"adc: no ADC_VERSION in {CONTROLLER} {ctl_ver} Makefile -- the pin moved, "
             "read config/manager/manager.yaml and update this checker")
pinned = m.group(1)

print(f"  pinned by {CONTROLLER} {ctl_ver} (Makefile ADC_VERSION): {pinned}")
tags = [t for t in github_tags("api7/adc") if re.match(r'^\d+\.\d+\.\d+$', t)]
if tags:
    newest = max(tags, key=vkey)
    note = "same" if newest == pinned else "AHEAD of the pin -- needs a controller bump too"
    print(f"  api7/adc newest tag (informational): {newest} ({note})")

report("adc", [pinned], n=1)
