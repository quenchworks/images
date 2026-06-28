#!/usr/bin/env python3
# authelia: GitHub release asset (authelia/authelia), tag vX.Y.Z. FROM_SOURCE=0
# but the melange fetches the upstream release binary, so the source is GitHub.
from _lib import github, report
report("authelia", github("authelia/authelia"))
