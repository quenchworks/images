#!/usr/bin/env python3
# trino: GitHub releases, tag NNN (one release line). Ships the latest only.
from _lib import github, report
report("trino", github("trinodb/trino"), n=1)
