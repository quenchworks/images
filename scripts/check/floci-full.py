#!/usr/bin/env python3
# floci-full: same upstream as apps/floci -- GitHub releases on floci-io/floci
# (tags have NO leading "v"). This app ships a single version (build.conf
# VERSIONS has one entry) while apps/floci ships three, hence n=1 here.
from _lib import github, report
report("floci-full", github("floci-io/floci"), n=1)
