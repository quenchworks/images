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
from _lib import github, json_get, report

tag = github("coollabsio/coolify")[0]
vj = json_get(f"https://raw.githubusercontent.com/coollabsio/coolify/v{tag}/versions.json")
rt = next(o["realtime"]["version"] for o in vj.values() if isinstance(o, dict) and "realtime" in o)
report("coolify-realtime", [rt], n=1)
