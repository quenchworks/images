#!/usr/bin/env python3
# varnish: the varnishd binary is Wolfi's flat `varnish` apk (FROM_SOURCE=0), so
# WOLFI is what the pin can actually follow -- that is the have-vs-latest check.
# Upstream is printed as a second, informational line because the two diverge
# (Wolfi's newest 8 apk is 8.0.0 while upstream is already at 8.0.2), and because
# the upstream home MOVED: varnishcache/varnish-cache is archived at 8.0.0 and
# varnish-cache.org redirects to vinyl-cache.org; 9.0.x lives in varnish/varnish.
import re
from _lib import report_lines, github, wolfi, vkey

report_lines("varnish", wolfi("varnish"), 2)

# informational: upstream's newest patch per line, straight from varnish/varnish
up = {}
for tag in github("varnish/varnish"):
    # _lib.github() lstrip("v")s the tag, so "varnish-9.0.3" arrives as
    # "arnish-9.0.3" -- match both spellings rather than silently dropping every tag.
    v = re.sub(r'^v?arnish-', '', tag)
    if not re.match(r'\d', v):
        continue
    line = ".".join(re.findall(r'\d+', v)[:2])
    if line not in up or vkey(v) > vkey(up[line]):
        up[line] = v
print("  upstream (github.com/varnish/varnish): " +
      ", ".join(f"{k}->{up[k]}" for k in sorted(up, key=vkey, reverse=True)[:4]))
