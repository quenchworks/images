#!/usr/bin/env python3
# jenkins: build fetches get.jenkins.io/war-stable/<ver>/jenkins.war. LTS baselines
# (X.Y.Z); compare newest patch per baseline line.
from _lib import scrape, report_lines
report_lines("jenkins", scrape("https://get.jenkins.io/war-stable/", r'href="(\d+\.\d+\.\d+)/"'), 2)
