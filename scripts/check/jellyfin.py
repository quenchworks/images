#!/usr/bin/env python3
# jellyfin: the Wolfi jellyfin apk (FROM_SOURCE=0), newest only. Wolfi's index also lists
# 10.11.x revisions; the recipe ships 12.x. Upstream tags land in Wolfi within days.
from _lib import wolfi, report
report("jellyfin", wolfi("jellyfin"), n=1)
