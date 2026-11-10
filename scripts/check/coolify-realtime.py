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
from urllib.error import HTTPError

from _lib import github, json_get, report

# 2026-11-03: 4.6.0 has no versions.json at all (404), so a release without the file is skipped too.
def pin(tag):
    try:
        vj = json_get(f"https://raw.githubusercontent.com/coollabsio/coolify/v{tag}/versions.json")
    except HTTPError:
        return None
    return next((o["realtime"]["version"] for o in vj.values() if isinstance(o, dict) and "realtime" in o), None)

# 2026-11-10: 4.4 onward ships no realtime at all, and ten 4.4+ releases pushed every 4.3 tag past a
# [:10] window (StopIteration). Walk all releases, skipping the lines known to drop it.
rt = next(v for v in map(pin, (t for t in github("coollabsio/coolify") if t.split(".")[:2] < ["4", "4"])) if v)
report("coolify-realtime", [rt], n=1)
