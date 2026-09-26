#!/usr/bin/env python3
# fluentd: GitHub releases (fluent/fluentd), tag vX.Y.Z. We ship the 1.19 line (see
# apps/fluentd/build.conf); a newer line shows as NEW LINE.
from _lib import github, report_lines
report_lines("fluentd", github("fluent/fluentd"), 2)
