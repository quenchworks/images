#!/usr/bin/env python3
# istio-proxyv2: version-coupled to istiod (istio/istio releases, bare tags). A line ships
# only once the Wolfi istio-envoy apk reaches that patch (see build.conf), so this tracks
# the newest line only; add depth when a second line's apk catches up.
from _lib import github, report_lines
report_lines("istio-proxyv2", github("istio/istio"), 1)
