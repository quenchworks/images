#!/usr/bin/env python3
# adminer: GitHub release asset adminer-<ver>.php (vrana/adminer), tag vX.Y.Z.
# Not Wolfi — FROM_SOURCE is unset but the melange fetches the release asset.
from _lib import github, report
report("adminer", github("vrana/adminer"))
