#!/usr/bin/env python3
# pgpool: Pgpool-II has no GitHub Releases (the mirror only carries V4_7_2 git
# tags). The canonical list is the "Stable versions" table on the download page,
# which is also where build.conf fetches the tarballs from. Same discovery as the
# recipe: newest patch of the latest 3 stable MINOR lines.
from _lib import scrape, report_lines

report_lines("pgpool",
             scrape("https://pgpool.github.io/download/source/",
                    r'pgpool-II-(\d+\.\d+\.\d+)\.tar\.gz'),
             2)
