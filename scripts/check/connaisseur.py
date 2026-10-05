#!/usr/bin/env python3
# connaisseur: GitHub releases, tag vX.Y.Z. One entry per minor line.
from _lib import github, report_lines
report_lines("connaisseur", github("sse-secure-systems/connaisseur"), 2)
