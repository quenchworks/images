#!/usr/bin/env python3
# grype: GitHub releases (anchore/grype).
from _lib import github, report
report("grype", github("anchore/grype"))
