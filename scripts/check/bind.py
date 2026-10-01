#!/usr/bin/env python3
# bind: Wolfi's bind apk (the 9.20 stable line), which the image pins.
from _lib import wolfi, report
report("bind", wolfi("bind"), n=1)
