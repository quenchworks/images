#!/usr/bin/env python3
# kyverno: GitHub releases (kyverno/kyverno), tags vX.Y.Z. NEWEST-ONLY, and that
# is deliberate: build.conf notes older minor lines pin transitive deps below
# their CVE fixes and cannot pass the 0-CVE matrix, so only the newest is built.
from _lib import github, report
report("kyverno", github("kyverno/kyverno"), n=1)
