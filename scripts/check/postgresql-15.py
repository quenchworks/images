#!/usr/bin/env python3
# postgresql-15: pinned to the 15.x line (ftp.postgresql.org source listing).
from _lib import scrape, report
report("postgresql-15", scrape("https://ftp.postgresql.org/pub/source/", r'href="v(15\.\d+)/"'))
