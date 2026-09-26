#!/usr/bin/env python3
# buildkit: GitHub releases, tag vX.Y.Z (dockerfile/* frontend tags are skipped by
# the semver filter). Window = latest patch of last 3 minor lines.
from _lib import github, report
report("buildkit", github("moby/buildkit"))
