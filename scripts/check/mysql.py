#!/usr/bin/env python3
# mysql: no clean GitHub (tags are MySQL Cluster). endoflife.date tracks the latest
# patch per MySQL cycle (8.0 / 8.4 / 9.x) — the same lines we ship.
from _lib import json_get, report_lines
cy = json_get("https://endoflife.date/api/mysql.json")
report_lines("mysql", [c["latest"] for c in cy if c.get("latest")], 2)
