#!/usr/bin/env python3
# mediawiki: release tarballs on releases.wikimedia.org/mediawiki/<line>/, the same
# place the recipe fetches from. Lines are listed at the top level; each line's
# directory holds mediawiki-X.Y.Z.tar.gz. We ship 1.43 (LTS) plus the supported
# stable lines, so report_lines compares per X.Y line; an obsolete line older than
# our newest (1.44) is never reported as new.
from _lib import scrape, report_lines, vkey
BASE = "https://releases.wikimedia.org/mediawiki/"
lines = sorted(set(scrape(BASE, r'href="(1\.\d+)/"')), key=vkey)[-4:]
cands = []
for line in lines:
    cands += scrape(f"{BASE}{line}/", r'mediawiki-(\d+\.\d+\.\d+)\.tar\.gz"')
report_lines("mediawiki", cands, depth=2)
