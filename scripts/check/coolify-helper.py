#!/usr/bin/env python3
# coolify-helper: independent 1.x line. Coolify keeps the pin in upstream.json (helper.version)
# since 4.6.0; versions.json, where it lived before, is gone from main.
from _lib import json_get, report
d = json_get("https://raw.githubusercontent.com/coollabsio/coolify/main/upstream.json")
report("coolify-helper", [d["helper"]["version"]])
