#!/usr/bin/env python3
# tomee (Apache TomEE): build fetches archive.apache.org/dist/tomee/. Newest
# stable line only; milestones (11.0.0-M1) are skipped by the pattern.
from _lib import scrape, report

cands = scrape("https://archive.apache.org/dist/tomee/",
               r'href="tomee-(\d+\.\d+\.\d+)/"')

report("tomee", cands, n=1)
