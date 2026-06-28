#!/usr/bin/env python3
# clickhouse: GitHub releases tagged vYY.M.P.B-stable / -lts. Strip the channel
# suffix; we ship the stable line, so drop -lts tags before comparing.
from _lib import github, report
tags = [t.split("-")[0] for t in github("ClickHouse/ClickHouse") if "-stable" in t]
report("clickhouse", tags)
