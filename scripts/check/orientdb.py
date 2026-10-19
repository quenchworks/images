#!/usr/bin/env python3
# orientdb: orientdb-community dist on Maven Central (X.Y.Z). We keep the two newest
# patch releases of the newest line, so lines are keyed per patch: a new release
# shows up as NEW LINE.
from _lib import scrape, report_lines
report_lines("orientdb", scrape("https://repo1.maven.org/maven2/com/orientechnologies/orientdb-community/maven-metadata.xml",
                                r"<version>(\d+\.\d+\.\d+)</version>"), 3)
