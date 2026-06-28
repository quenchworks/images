#!/usr/bin/env python3
# elixir: Wolfi has no bare elixir-<minor>; it's provided by erl<N>-elixir-<minor>
# (e.g. erl28-elixir-1.19). Take the newest elixir version per elixir-minor line
# across the erlN- variants, compare to our pinned lines.
from _lib import wolfi_match, current, vkey
import re
m = wolfi_match(r"^erl\d+-elixir-\d+\.\d+$")
lines = {}
for pkg, ver in m.items():
    lk = ".".join(re.findall(r"\d+", ver)[:2])
    if lk not in lines or vkey(ver) > vkey(lines[lk]):
        lines[lk] = ver
cur = current("elixir"); behind = []
print("elixir")
for c in cur:
    lk = ".".join(re.findall(r"\d+", c)[:2])
    w = lines.get(lk)
    upd = bool(w) and vkey(w) > vkey(c)
    if upd: behind.append(w)
    print(f"  elixir-{lk}: have={c:>10s}  latest={(w or '?'):>10s}  {'UPDATE' if upd else 'ok'}")
print(f"  => {'UPDATE -> ' + str(behind) if behind else 'ok'}")
