#!/usr/bin/env python3
# ghost: npm package (ghost), installed onto the Node runtime. Drop prereleases.
from _lib import npm, report
report("ghost", npm("ghost"), keep=lambda v: "-" not in v)
