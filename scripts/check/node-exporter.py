#!/usr/bin/env python3
# node-exporter: GitHub releases (prometheus/node_exporter).
from _lib import github, report
report("node-exporter", github("prometheus/node_exporter"))
