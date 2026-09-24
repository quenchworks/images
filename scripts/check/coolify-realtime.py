#!/usr/bin/env python3
# coolify-realtime: independent 1.x line, read from the published image tags.
#
# It used to read coolify.realtime.version out of upstream's versions.json.
# That key is GONE as of 2026-09-24; the file now carries only v4, nightly,
# helper and sentinel, so the checker died with KeyError: 'realtime' and stopped
# reporting anything. The component itself is very much alive: coollabsio
# publishes coolify-realtime image tags, which is the source of truth now.
#
# Plain X.Y.Z only. The repository also carries latest, next, and per-arch
# suffixes (1.0.20-amd64, 1.0.20-aarch64) that would otherwise sort into the
# window as separate versions.
import re
from _lib import json_get, report

d = json_get("https://hub.docker.com/v2/repositories/coollabsio/coolify-realtime/tags?page_size=100")
tags = [t["name"] for t in (d.get("results") or [])]
report("coolify-realtime", [t for t in tags if re.fullmatch(r"\d+\.\d+\.\d+", t)], n=1)
