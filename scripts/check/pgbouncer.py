#!/usr/bin/env python3
# pgbouncer: build fetches pgbouncer.org; versions = GitHub releases tagged
# "pgbouncer_1_25_2" (pgbouncer/pgbouncer).
from _lib import github, report
report("pgbouncer", github("pgbouncer/pgbouncer"),
       clean=lambda t: t.replace("pgbouncer_", "").replace("_", "."))
