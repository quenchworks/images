#!/usr/bin/env python3
# strimzi-kafka: versions are <strimzi>-kafka-<kafka>, where <kafka> is the
# default in that Strimzi release's kafka-versions.yaml; a new Strimzi release
# or a new default Kafka both show up as an update.
import re, urllib.request
from _lib import github, report, vkey
tags = [t for t in github("strimzi/strimzi-kafka-operator") if t[:1].isdigit()]
newest = max(tags, key=vkey) if tags else None
cands = []
if newest:
    url = f"https://raw.githubusercontent.com/strimzi/strimzi-kafka-operator/{newest}/kafka-versions.yaml"
    text = urllib.request.urlopen(url, timeout=30).read().decode()
    # stdlib only: one "- version:" block per Kafka release, one marked default
    for block in re.split(r"\n- ", text):
        m = re.search(r"version:\s*(\S+)", block)
        if m and re.search(r"^\s*default:\s*true", block, re.M):
            cands.append(f"{newest}-kafka-{m.group(1)}")
report("strimzi-kafka", cands, n=1)
