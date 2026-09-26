#!/usr/bin/env python3
# n8n: npm package (n8n), installed onto the Node runtime. NEWEST-ONLY: we ship only the
# newest stable minor line (older lines pin Node deps below their CVE fixes and fail the
# comprehensive gate), so build.conf VERSIONS is a single entry and report_lines — which
# iterates the current entry — reports just the line we ship.
# npm carries 2.29.x versions with no "-" suffix, but those are the `next`/`beta` line — not
# yet promoted to `stable`/`latest`. Gate on the `stable` dist-tag's minor line so a beta
# minor never shows as an UPDATE; report_lines then tracks the newest patch on the line we ship.
from _lib import report_lines, vkey, json_get

data = json_get("https://registry.npmjs.org/n8n")
stable = data["dist-tags"]["stable"]                       # e.g. 2.28.5
stable_line = vkey(".".join(stable.split(".")[:2]))        # (2, 28)
cands = [v for v in data["versions"]
         if "-" not in v and vkey(".".join(v.split(".")[:2])) <= stable_line]
report_lines("n8n", cands, depth=2)
