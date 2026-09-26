#!/usr/bin/env python3
# activemq: ActiveMQ Classic releases on downloads.apache.org. We ship the 6.3 line (see
# apps/activemq/build.conf); a newer line shows as NEW LINE.
from _lib import scrape, report_lines
report_lines("activemq", scrape("https://downloads.apache.org/activemq/", r'href="([56]\.[0-9]+\.[0-9]+)/"'), 2)
