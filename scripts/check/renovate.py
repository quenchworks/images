#!/usr/bin/env python3
# renovate: npm package. npm lists prerelease "44.0.0-next.N" tags; we track
# stable, so drop any version with a "-" suffix.
from _lib import npm, report
report("renovate", npm("renovate"), keep=lambda v: "-" not in v)
