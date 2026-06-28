#!/usr/bin/env python3
# node: tracks Wolfi's nodejs-<major> apks (NOT nodejs.org), one LTS line each.
# SPECIAL: the window is 4 DISTINCT major lines, so compare each major's newest
# Wolfi patch against our current entry for that same major (not a global top-N).
from _lib import wolfi, current, vkey
cur = {v.split(".")[0]: v for v in current("node")}
print("node")
behind = []
for maj in (20, 22, 24, 26):
    newest = sorted(wolfi(f"nodejs-{maj}"), key=vkey, reverse=True)[0]
    have = cur.get(str(maj), "?")
    upd = have == "?" or vkey(newest) > vkey(have)
    if upd:
        behind.append(newest)
    print(f"  nodejs-{maj}: have={have:>10s}  latest={newest:>10s}  {'UPDATE' if upd else 'ok'}")
print(f"  => {'UPDATE -> ' + str(behind) if behind else 'ok'}")
