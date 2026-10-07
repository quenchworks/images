#!/usr/bin/env python3
# coolify-realtime: the realtime version Coolify's newest release pins in
# versions.json. Docker Hub also carries tags no Coolify release installs
# (1.0.19-next, 1.0.20), so the release pin is what "latest" means here.
#
# History: on 2026-09-24 the realtime key vanished from versions.json on Coolify's
# main branch (main replaced this image with Reverb plus coolify-terminal), and the
# checker then read Docker Hub tags instead. Reading the file at the newest RELEASE
# tag gets the pin again. When a release drops the key, next() raises instead of
# reporting a stale "ok": that release no longer ships this image.
# 2026-10-07: Coolify 4.4.0 dropped the key (realtime replaced by Reverb + coolify-terminal),
# so this reads the newest release that still pins it. coolify-app 4.4.0 is held for that
# redesign; while it is, this pin cannot move.
from _lib import github, json_get, report

def pin(tag):
    vj = json_get(f"https://raw.githubusercontent.com/coollabsio/coolify/v{tag}/versions.json")
    return next((o["realtime"]["version"] for o in vj.values() if isinstance(o, dict) and "realtime" in o), None)

rt = next(v for v in map(pin, github("coollabsio/coolify")[:10]) if v)
report("coolify-realtime", [rt], n=1)
