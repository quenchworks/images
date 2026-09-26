#!/usr/bin/env python3
# actions-runner: GitHub releases (actions/runner), tags vX.Y.Z. Latest only: GitHub
# stops accepting old runner versions after about 30 days.
from _lib import github, report
report("actions-runner", github("actions/runner"), n=1)
