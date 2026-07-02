#!/usr/bin/env python3
# n8n-runners: the external task-runner sidecar, versioned in LOCKSTEP with n8n (the image
# tag == the n8n version it pairs with). So we track the exact same source as n8n.py: the
# npm `n8n` package's `stable` dist-tag line, newest patch of each of the last 3 stable MINOR
# lines. (The bundled component versions — @n8n/task-runner, task-runner-launcher, the python
# runner — are pinned per n8n version inside apps/n8n-runners/build.conf.)
from _lib import report_lines, vkey, json_get

data = json_get("https://registry.npmjs.org/n8n")
stable = data["dist-tags"]["stable"]                       # e.g. 2.28.5
stable_line = vkey(".".join(stable.split(".")[:2]))        # (2, 28)
cands = [v for v in data["versions"]
         if "-" not in v and vkey(".".join(v.split(".")[:2])) <= stable_line]
report_lines("n8n-runners", cands, depth=2)
