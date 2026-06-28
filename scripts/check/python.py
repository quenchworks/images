#!/usr/bin/env python3
# python: Wolfi apk (FROM_SOURCE=0). Window = latest patch per line in build.conf.
from _lib import wolfi, report
report("python", wolfi("python"))
