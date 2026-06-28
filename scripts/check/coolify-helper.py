#!/usr/bin/env python3
# coolify-helper: independent 1.x line; upstream versions.json -> coolify.helper.version.
from _lib import json_get, report
d = json_get("https://raw.githubusercontent.com/coollabsio/coolify/main/versions.json")
report("coolify-helper", [d["coolify"]["helper"]["version"]])
