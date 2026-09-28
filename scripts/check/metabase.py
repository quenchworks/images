#!/usr/bin/env python3
# metabase: GitHub releases of metabase/metabase. The open-source edition is
# tagged v0.X.Y (v1.X.Y is Enterprise), so only v0 tags count. Latest only.
from _lib import github, report
report("metabase", [t for t in github("metabase/metabase") if t.startswith("0.")], n=1)
