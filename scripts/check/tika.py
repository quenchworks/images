#!/usr/bin/env python3
# tika: Apache Tika releases on archive.apache.org. Newest major line only, per build.conf.
from _lib import scrape, report
report("tika", scrape("https://archive.apache.org/dist/tika/", r'href="(\d+\.\d+\.\d+)/"'), n=1)
