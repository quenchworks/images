#!/usr/bin/env python3
# gitness: GitHub releases (harness/gitness), tags vX.Y.Z. NEWEST-ONLY per
# build.conf. Upstream also keeps patching an older v2 line, but vkey sorting
# puts the v3 line on top, which is the line the recipe tracks.
from _lib import github, report
report("gitness", github("harness/gitness"), n=1)
