#!/usr/bin/env python3
# centrifugo: GitHub releases (centrifugal/centrifugo), tags vX.Y.Z. build.conf
# ships one version (the newest patch of the newest line), so n=1.
from _lib import github, report
report("centrifugo", github("centrifugal/centrifugo"), n=1)
