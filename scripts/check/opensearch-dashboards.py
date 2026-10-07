#!/usr/bin/env python3
# opensearch-dashboards: GitHub releases (opensearch-project/OpenSearch-Dashboards). Lines
# must match the OpenSearch ones we ship.
from _lib import github, report
report("opensearch-dashboards", github("opensearch-project/OpenSearch-Dashboards"), n=1)
