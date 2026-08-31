#!/usr/bin/env python3
# perses: GitHub releases (perses/perses), tags vX.Y.Z. build.conf ships one
# version, so n=1.
from _lib import github, report
report("perses", github("perses/perses"), n=1)
