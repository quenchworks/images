#!/usr/bin/env python3
# pinniped: GitHub releases (vmware/pinniped), tag vX.Y.Z. LATEST ONLY: upstream never
# patches an older minor (see apps/pinniped/build.conf).
from _lib import github, report
report("pinniped", github("vmware/pinniped"), n=1)
