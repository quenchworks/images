#!/usr/bin/env python3
# poetry: flat Wolfi apk 'py3.13-poetry-bin' — latest patch stream.
from _lib import wolfi, report
report("poetry", wolfi("py3.13-poetry-bin"))
