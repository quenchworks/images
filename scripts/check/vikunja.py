#!/usr/bin/env python3
# vikunja: GitHub releases (go-vikunja/vikunja, the mirror of code.vikunja.io).
# Was MISSING until 2026-08-31, which is why vikunja sat two releases behind
# (2.3.0 vs 2.5.0) carrying 2 HIGH + 3 MEDIUM without ever showing up as an
# UPDATE -- check-updates.py globs THIS directory, so no file means no check.
from _lib import github, report
report("vikunja", github("go-vikunja/vikunja"))
