#!/usr/bin/env python3
# coolify-app: coolify itself; upstream versions.json -> coolify.v4.version.
from _lib import json_get, report
d = json_get("https://raw.githubusercontent.com/coollabsio/coolify/main/versions.json")
report("coolify-app", [d["coolify"]["v4"]["version"]])
