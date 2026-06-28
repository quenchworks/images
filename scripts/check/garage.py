#!/usr/bin/env python3
# garage: hosted on Deuxfleurs' Gitea (git.deuxfleurs.fr), not GitHub. Use its API.
from _lib import json_get, report_lines
tags = [t["name"].lstrip("v") for t in json_get("https://git.deuxfleurs.fr/api/v1/repos/Deuxfleurs/garage/tags?limit=50")]
report_lines("garage", tags, 2)
