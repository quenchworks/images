#!/usr/bin/env python3
# redmine: release tarballs on www.redmine.org/releases (redmine-X.Y.Z.tar.gz). Newest only.
from _lib import scrape, report
report("redmine", scrape("https://www.redmine.org/releases/", r'redmine-([0-9]+\.[0-9]+\.[0-9]+)\.tar\.gz'), n=1)
