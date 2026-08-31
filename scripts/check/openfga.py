#!/usr/bin/env python3
# openfga: GitHub releases (openfga/openfga), tags vX.Y.Z. build.conf ships one
# version, so n=1.
from _lib import github, report
report("openfga", github("openfga/openfga"), n=1)
