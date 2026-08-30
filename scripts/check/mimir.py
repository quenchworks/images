#!/usr/bin/env python3
# mimir: GitHub releases (grafana/mimir), tag prefix "mimir-".
from _lib import github, report
report("mimir", [t.removeprefix("mimir-") for t in github("grafana/mimir")])
