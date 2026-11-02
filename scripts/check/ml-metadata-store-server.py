#!/usr/bin/env python3
# ml-metadata-store-server: held (see build.conf). Wolfi dropped the apk (2026-11-02), so watch
# google/ml-metadata releases: an unblock now means a from-source build.
from _lib import github, report
report("ml-metadata-store-server", github("google/ml-metadata"), n=1)
