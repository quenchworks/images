#!/usr/bin/env python3
# jetty: Maven Central (org.eclipse.jetty:jetty-home). Supported lines 12.1 and 12.0,
# newest patch of each (11 is past community support).
from _lib import scrape, report_lines
report_lines("jetty", scrape("https://repo1.maven.org/maven2/org/eclipse/jetty/jetty-home/maven-metadata.xml", r"<version>(12\.[0-9.]+)</version>"), 2)
