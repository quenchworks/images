#!/usr/bin/env python3
# coolify-app: coolify itself; GitHub releases, stable v4 tags only. main's
# versions.json names the NEXT version before it is tagged (it said 4.4 while
# v4.3.23 was the newest release), and the recipe checks out a release commit.
import re
from _lib import github, report
report("coolify-app", [t for t in github("coollabsio/coolify") if re.fullmatch(r"4\.\d+\.\d+", t)], n=1)
