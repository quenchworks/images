#!/usr/bin/env python3
# mediamtx: GitHub releases (bluenviron/mediamtx), tags vX.Y.Z. build.conf ships
# a single newest version, so n=1.
from _lib import github, report
report("mediamtx", github("bluenviron/mediamtx"), n=1)
