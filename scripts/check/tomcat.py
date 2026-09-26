#!/usr/bin/env python3
# tomcat (Apache Tomcat): build fetches downloads.apache.org/tomcat/tomcat-11.
# 11.0.x is the current GA line, so we ship the NEWEST 11.0 patch only. Scrape
# the tomcat-11 release-directory listing for v11.0.<p>/ entries and report the
# newest one.
from _lib import scrape, report

cands = scrape("https://downloads.apache.org/tomcat/tomcat-11/",
               r'href="v(11\.0\.\d+)/"')

report("tomcat", cands, n=1)
