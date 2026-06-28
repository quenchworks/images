#!/usr/bin/env python3
# busybox: build fetches busybox.net/downloads/busybox-<ver>.tar.bz2 (no GitHub).
from _lib import scrape, report_lines
report_lines("busybox", scrape("https://busybox.net/downloads/", r'busybox-(\d+\.\d+\.\d+)\.tar\.bz2'), 2)
