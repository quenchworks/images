#!/usr/bin/env python3
# cockroachdb: no GitHub Releases, only tags vYY.M.P (+ -alpha/-beta/-rc
# prereleases we must drop). Source binary tarball is pinned per stable tag.
from _lib import github_tags, report
report("cockroachdb", github_tags("cockroachdb/cockroach"), keep=lambda v: "-" not in v)
