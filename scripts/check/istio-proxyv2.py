#!/usr/bin/env python3
# istio-proxyv2: version-coupled to istiod, so it tracks the same istio/istio
# releases (bare tags, no "v" prefix). BLOCKED (see build.conf) -- this checker
# still reports have-vs-latest so the block gets revisited when upstream moves,
# same as any other tracked app.
from _lib import github, report_lines
report_lines("istio-proxyv2", github("istio/istio"), 2)
