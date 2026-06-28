#!/usr/bin/env python3
# pnpm: from-source from the npm registry (pnpm). Per major line (9/10/11); drop
# prerelease "-next" tags.
from _lib import npm, report_lines
report_lines("pnpm", npm("pnpm"), 1, keep=lambda v: "-" not in v)
