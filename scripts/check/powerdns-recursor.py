#!/usr/bin/env python3
# powerdns-recursor: PowerDNS Recursor releases, the rec-X.Y.Z tags of
# PowerDNS/pdns (the same repo carries auth- and dnsdist- tags on other lines).
# Two lines, checked per line against build.conf.
import re
from _lib import _gh, report_lines

refs = _gh("repos/PowerDNS/pdns/git/matching-refs/tags/rec-", ".[].ref")
tags = [r[len("refs/tags/rec-"):] for r in refs]
report_lines("powerdns-recursor", [t for t in tags if re.fullmatch(r"\d+\.\d+\.\d+", t)], 2)
