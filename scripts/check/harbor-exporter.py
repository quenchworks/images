#!/usr/bin/env python3
# harbor-exporter: GitHub releases (goharbor/harbor).
from _lib import github, report
report("harbor-exporter", github("goharbor/harbor"))
