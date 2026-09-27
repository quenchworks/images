#!/usr/bin/env python3
# squid: the squid binary is Wolfi's flat `squid` apk (FROM_SOURCE=0), so Wolfi is
# what the pin can follow. Latest only: Squid supports one release line at a time.
from _lib import report_lines, wolfi

report_lines("squid", wolfi("squid"), 1)
