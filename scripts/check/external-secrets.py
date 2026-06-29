#!/usr/bin/env python3
# external-secrets: GitHub releases (external-secrets/external-secrets). The repo
# also cuts `helm-chart-X.Y.Z` release tags for the chart; report() drops those
# automatically (they don't start with a digit after v-strip).
from _lib import github, report
report("external-secrets", github("external-secrets/external-secrets"))
