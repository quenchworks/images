#!/usr/bin/env python3
# spark: Wolfi versioned apk spark-<X.Y> (FROM_SOURCE=0, built from source by Wolfi).
# depth=2 compares each shipped line against Wolfi's newest patch and flags a new
# line (4.2) once Wolfi packages it.
from _lib import report_wolfi_lines
report_wolfi_lines("spark", "spark-", 2)
