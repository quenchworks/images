#!/usr/bin/env python3
# frr: the Wolfi frr-10.6 apk (FROM_SOURCE=0), newest line only.
from _lib import wolfi, report
report("frr", wolfi("frr-10.6"), n=1)
