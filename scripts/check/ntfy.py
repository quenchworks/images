#!/usr/bin/env python3
# ntfy: GitHub releases (binwiederhier/ntfy), tags vX.Y.Z. NEWEST-ONLY -- build.conf
# is the single source of truth and lists just the newest stable tag.
from _lib import github, report
report("ntfy", github("binwiederhier/ntfy"), n=1)
