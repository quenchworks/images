#!/usr/bin/env python3
# jaeger: GitHub releases (jaegertracing/jaeger). The repo releases BOTH trains
# from one tag namespace -- v1.x (the legacy collector/query binaries) and v2.x
# (the single OTel-Collector-based binary this recipe builds). Verified on the
# releases API: v2.20.0 and v1.76.0 coexist. Only the v2 line is buildable by
# this recipe, so keep majors == 2. NEWEST-ONLY per build.conf.
from _lib import github, report
report("jaeger", github("jaegertracing/jaeger"), n=1,
       keep=lambda t: t.startswith("2."))
