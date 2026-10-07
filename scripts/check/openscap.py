#!/usr/bin/env python3
# openscap: the Wolfi openscap apk (FROM_SOURCE=0), newest only; scap-security-guide is pinned beside it.
from _lib import wolfi, report
report("openscap", wolfi("openscap"), n=1)
