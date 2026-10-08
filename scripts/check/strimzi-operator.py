#!/usr/bin/env python3
# strimzi-operator: built from the GitHub source tag (tags have no leading `v`;
# release candidates are GitHub prereleases and are skipped). Newest line only.
from _lib import github, report
report("strimzi-operator", github("strimzi/strimzi-kafka-operator"), n=1)
