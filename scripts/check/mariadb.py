#!/usr/bin/env python3
# mariadb: build fetches archive.mariadb.org/mariadb-<ver>/. Per maintained line.
from _lib import scrape, report_lines
report_lines("mariadb", scrape("https://archive.mariadb.org/", r'mariadb-(\d+\.\d+\.\d+)/'), 2)
