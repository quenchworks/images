#!/usr/bin/env python3
# zot: GitHub releases (project-zot/zot), tags vX.Y.Z. build.conf ships one
# version off the only actively-maintained 2.1 line, so n=1.
from _lib import github, report
report("zot", github("project-zot/zot"), n=1)
