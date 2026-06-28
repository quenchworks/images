#!/usr/bin/env python3
# grafana: build fetches dl.grafana.com; versions = GitHub releases (grafana/grafana).
from _lib import github, report
report("grafana", github("grafana/grafana"))
