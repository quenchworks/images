#!/usr/bin/env python3
# crane: the unversioned Wolfi crane apk (FROM_SOURCE=0), newest-only like the recipe.
from _lib import wolfi, report
report("crane", wolfi("crane"), n=1)
