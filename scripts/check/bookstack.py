#!/usr/bin/env python3
# bookstack: GitHub releases on BookStackApp/BookStack, tags vYY.MM.N. LATEST-ONLY.
from _lib import github, report
report("bookstack", github("BookStackApp/BookStack"), n=1)
