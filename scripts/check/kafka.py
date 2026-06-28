#!/usr/bin/env python3
# kafka: build fetches archive.apache.org/dist/kafka/<ver>/. Available versions =
# the current-releases dir listing on downloads.apache.org. Per minor line.
from _lib import scrape, report_lines
report_lines("kafka", scrape("https://downloads.apache.org/kafka/", r'href="(\d+\.\d+\.\d+)/"'), 2)
