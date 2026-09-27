#!/usr/bin/env python3
# powerdns: PowerDNS AUTHORITATIVE releases, the auth-X.Y.Z tags of PowerDNS/pdns.
# That repo also carries rec-X.Y.Z (recursor) and dnsdist tags on other version
# lines, and the 40 newest tags are often all rec-, so this asks for auth- refs
# directly. Two lines, matching build.conf.
import re
from _lib import _gh, report

refs = _gh("repos/PowerDNS/pdns/git/matching-refs/tags/auth-", ".[].ref")
tags = [r[len("refs/tags/auth-"):] for r in refs]
report("powerdns", [t for t in tags if re.fullmatch(r"\d+\.\d+\.\d+", t)], n=2)
