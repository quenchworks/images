#!/usr/bin/env python3
# storm (Apache Storm): build fetches archive.apache.org/dist/storm/. Newest line only.
from _lib import scrape, report

cands = scrape("https://archive.apache.org/dist/storm/",
               r'href="apache-storm-(\d+\.\d+\.\d+)/"')

report("storm", cands, n=1)
