#!/usr/bin/env python3
# pulsar: build fetches archive.apache.org; versions = GitHub releases (apache/pulsar).
from _lib import github, report
report("pulsar", github("apache/pulsar"))
