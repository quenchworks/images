#!/usr/bin/env python3
# kured: GitHub releases (kubereboot/kured); helm-chart tags live in a separate repo.
from _lib import github, report_lines
report_lines("kured", github("kubereboot/kured"), 2)
