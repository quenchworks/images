#!/usr/bin/env python3
# rqlite: GitHub releases (rqlite/rqlite), tags vX.Y.Z. build.conf ships one
# version (the newest patch of the newest line), so n=1.
from _lib import github, report
report("rqlite", github("rqlite/rqlite"), n=1)
