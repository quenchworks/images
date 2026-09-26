#!/usr/bin/env python3
# httpd (Apache HTTP Server): build fetches downloads.apache.org/httpd. Apache
# maintains a SINGLE stable line (2.4.x) with no parallel 2.2/2.6, so we ship the
# NEWEST 2.4 patch only. Scrape the release directory listing for httpd-2.4.<p>
# source tarballs and report the newest one.
from _lib import scrape, report


cands = scrape("https://downloads.apache.org/httpd/",
               r'httpd-(2\.4\.\d+)\.tar\.bz2(?!\.)')

report("httpd", cands, n=1)
