#!/usr/bin/env python3
# postgresql: build fetches ftp.postgresql.org/pub/source/v<ver>/. Versions = that
# listing; we ship one entry per MAJOR line (16/17/18), so compare per major.
from _lib import scrape, report_lines
report_lines("postgresql", scrape("https://ftp.postgresql.org/pub/source/", r'href="v(\d+\.\d+(?:\.\d+)?)/"'), 1)
