#!/usr/bin/env python3
# goaccess: git tags vX.Y[.Z] on allinurl/goaccess (no GitHub release objects). LATEST-ONLY.
from _lib import github_tags, report
report("goaccess", github_tags("allinurl/goaccess"), n=1)
