#!/usr/bin/env python3
# vouch-proxy: git tags vX.Y.Z on vouch/vouch-proxy (no GitHub releases). LATEST-ONLY.
from _lib import github_tags, report
report("vouch-proxy", github_tags("vouch/vouch-proxy"), n=1)
