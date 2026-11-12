#!/usr/bin/env python3
# actual: the @actual-app/sync-server npm package (what the recipe installs), stable releases only
# (nightlies carry a -nightly suffix). LATEST-ONLY.
from _lib import npm, report
report("actual", npm("@actual-app/sync-server"), n=1, keep=lambda v: "-" not in v)
