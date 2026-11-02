#!/usr/bin/env python3
# cockroachdb: the recipe fetches binaries.cockroachdb.com tarballs, and that CDN is the only
# complete source: 26.3.x has no git tag on cockroachdb/cockroach (tags stop at 26.2.10) and the
# docs releases.yml lags (26.3.1 while 26.3.4 shipped), measured 2026-11-02. So probe the CDN for
# the next patches of every shipped line, the next two minors and next year's .1.
# ponytail: probes 5 patches ahead; a line that skips more needs a wider range.
import re, urllib.request
from _lib import current, report

def exists(v):
    req = urllib.request.Request(f"https://binaries.cockroachdb.com/cockroach-v{v}.linux-amd64.tgz", method="HEAD")
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status == 200
    except Exception:
        return False

cands = set()
for v in current("cockroachdb"):
    y, m, p = (int(x) for x in re.findall(r"\d+", v)[:3])
    cands.update(f"{y}.{m}.{q}" for q in range(p, p + 6))
    cands.update(f"{y}.{m + k}.{q}" for k in (1, 2) for q in range(0, 3))
    cands.update(f"{y + 1}.1.{q}" for q in range(0, 3))  # YY.1 opens each year
report("cockroachdb", [v for v in sorted(cands) if exists(v)])
