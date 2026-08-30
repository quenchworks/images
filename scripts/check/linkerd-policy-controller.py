#!/usr/bin/env python3
# linkerd-policy-controller: same upstream repo/tag stream as
# linkerd-control-plane (both build out of linkerd/linkerd2's edge-YY.M.P
# releases -- see that check script / linkerd-control-plane/build.conf for the
# stable-vs-edge cadence verification). Window = newest patch of the last 3
# edge year.month lines, matching the recipe's VERSIONS.
from _lib import github, report_lines
tags = [t[len("edge-"):] for t in github("linkerd/linkerd2") if t.startswith("edge-")]
report_lines("linkerd-policy-controller", tags, 2)
