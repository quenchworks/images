#!/usr/bin/env python3
# tyk: GitHub releases (TykTechnologies/tyk), tags vX.Y.Z. NEWEST-ONLY.
# Upstream marks its pre-release builds as NON-prerelease on the API (verified:
# v5.12.0-alphafips5 and v5.11.1-rc1 both come back with prerelease=false), so
# github()`s prerelease filter is not enough -- build.conf says no
# -alpha/-rc/-fips suffix, so drop any tag carrying a suffix.
from _lib import github, report
report("tyk", github("TykTechnologies/tyk"), n=1,
       keep=lambda t: "-" not in t)
