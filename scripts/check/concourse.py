#!/usr/bin/env python3
# concourse: GitHub releases of concourse/concourse, the tag archive the recipe fetches. Newest only.
from _lib import github, report
report("concourse", github("concourse/concourse"), n=1)
