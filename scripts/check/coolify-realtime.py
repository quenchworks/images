#!/usr/bin/env python3
# coolify-realtime: independent 1.x line; upstream versions.json -> coolify.realtime.version.
from _lib import json_get, report
d = json_get("https://raw.githubusercontent.com/coollabsio/coolify/main/versions.json")
report("coolify-realtime", [d["coolify"]["realtime"]["version"]])
