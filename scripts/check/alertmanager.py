#!/usr/bin/env python3
# alertmanager: GitHub releases (prometheus/alertmanager).
from _lib import github, report
report("alertmanager", github("prometheus/alertmanager"))
