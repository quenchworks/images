#!/usr/bin/env python3
# gradle: flat Wolfi apk 'gradle-9' — ship the latest clean line only (older lines
# lack the backported jackson-databind fix; see build.conf).
from _lib import wolfi, report
report("gradle", wolfi("gradle-9"), n=1)
