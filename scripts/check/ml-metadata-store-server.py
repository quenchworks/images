#!/usr/bin/env python3
# ml-metadata-store-server: the Wolfi apk (FROM_SOURCE=0), newest only. Held (see build.conf):
# also watch google/ml-metadata releases for one that moves its bundled C dependencies.
from _lib import wolfi, report
report("ml-metadata-store-server", wolfi("ml-metadata-store-server"), n=1)
