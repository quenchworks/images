#!/usr/bin/env python3
# blackbox-exporter: GitHub releases (prometheus/blackbox_exporter).
from _lib import github, report
report("blackbox-exporter", github("prometheus/blackbox_exporter"))
