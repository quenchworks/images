#!/usr/bin/env python3
# xwiki: xwiki-platform-distribution-war releases on maven.xwiki.org (X.Y.Z, RCs
# filtered). Two lines like the recipe: the X.10 LTS and the newest stable line.
from _lib import scrape, report_lines
report_lines("xwiki", scrape("https://maven.xwiki.org/releases/org/xwiki/platform/xwiki-platform-distribution-war/maven-metadata.xml",
                             r"<version>(\d+\.\d+\.\d+)</version>"), 2)
