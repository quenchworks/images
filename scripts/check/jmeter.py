#!/usr/bin/env python3
# jmeter: build fetches archive.apache.org; versions = GitHub releases tagged
# "rel/vX.Y.Z" (apache/jmeter).
from _lib import github, report
report("jmeter", github("apache/jmeter"), clean=lambda t: t.replace("rel/v", ""))
