#!/usr/bin/env python3
# activemq-artemis: Apache Artemis releases on dlcdn.apache.org/artemis/artemis/ (the
# project left ActiveMQ at 2.50). We ship the newest release; see build.conf.
from _lib import scrape, report
report("activemq-artemis", scrape("https://dlcdn.apache.org/artemis/artemis/", r'href="([0-9]+\.[0-9]+\.[0-9]+)/"'), n=1)
