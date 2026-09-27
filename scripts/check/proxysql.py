#!/usr/bin/env python3
# proxysql: the binary is Wolfi's flat `proxysql` apk (FROM_SOURCE=0), so Wolfi is
# what the pin can follow. Upstream's newest stable is printed as a second,
# informational line: Wolfi lags it by a patch or two. 3.1.x and 4.0.x are
# prereleases upstream, and github() drops them.
from _lib import report_lines, github, wolfi

report_lines("proxysql", wolfi("proxysql"), 1)
print("  upstream (github.com/sysown/proxysql): " + ", ".join(github("sysown/proxysql")[:3]))
