#!/usr/bin/env python3
# typesense: GitHub releases (typesense/typesense), tagged vXX.Y. We ship the 3
# newest stable major lines; _lib.report compares against what we have.
from _lib import github, report
report("typesense", github("typesense/typesense"), n=3)
