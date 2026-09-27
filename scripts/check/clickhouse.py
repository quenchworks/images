#!/usr/bin/env python3
# clickhouse: GitHub releases tagged vYY.M.P.B-stable / -lts. Strip the channel
# suffix; we ship the stable line, so drop -lts tags before comparing. Checked
# per line (YY.M), matching build.conf's one-patch-per-line window.
from _lib import github, report_lines
tags = [t.split("-")[0] for t in github("ClickHouse/ClickHouse") if "-stable" in t]
report_lines("clickhouse", tags, 2)
