#!/usr/bin/env python3
# hadolint: GitHub releases (hadolint/hadolint), tag vX.Y.Z. Latest only.
from _lib import github, report
report("hadolint", github("hadolint/hadolint"), n=1)
