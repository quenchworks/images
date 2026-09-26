#!/usr/bin/env python3
# shellcheck: GitHub releases (koalaman/shellcheck), tag vX.Y.Z. Latest only.
from _lib import github, report
report("shellcheck", github("koalaman/shellcheck"), n=1)
