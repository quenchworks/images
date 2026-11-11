#!/usr/bin/env python3
# trafficserver: Apache release tarballs on downloads.apache.org/trafficserver/. 10.x only:
# 10.1 still needs PCRE1, so the recipe ships the newest 10.2+ line (see build.conf).
from _lib import scrape, report
report("trafficserver", [v for v in scrape("https://downloads.apache.org/trafficserver/", r'trafficserver-(\d+\.\d+\.\d+)\.tar\.bz2"') if not v.startswith(("9.", "10.0.", "10.1."))], n=1)
