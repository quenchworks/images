#!/usr/bin/env python3
# friendica: the friendica-full release tarballs on files.friendi.ca, the same place the
# recipe fetches from (GitHub tags like 2026.05-1 can be CI-only re-tags with no tarball).
# Friendica keeps one stable line, so we ship the newest only; -dev and -rc are skipped.
from _lib import scrape, report
cands = scrape("https://files.friendi.ca/", r'friendica-full-(\d{4}\.\d{2}(?:-\d+)?)\.tar\.gz"')
report("friendica", cands, n=1)
