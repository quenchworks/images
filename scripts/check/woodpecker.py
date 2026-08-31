#!/usr/bin/env python3
# woodpecker: GitHub releases (woodpecker-ci/woodpecker), tags vX.Y.Z. build.conf
# ships one version, so n=1.
from _lib import github, report
report("woodpecker", github("woodpecker-ci/woodpecker"), n=1)
