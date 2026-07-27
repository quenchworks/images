#!/usr/bin/env python3
# jenkins-inbound-agent: Wolfi apk 'jenkins-docker-agent-openjdk-21' (FROM_SOURCE=0)
# — the agent script + agent.jar. Window = latest docker-agent versions in build.conf.
# Wolfi retired the flat 'jenkins-docker-agent' name; the per-JDK subpackages carry the
# version line now (-openjdk-17 is frozen at 2.544, so track -openjdk-21).
from _lib import wolfi, report
report("jenkins-inbound-agent", wolfi("jenkins-docker-agent-openjdk-21"))
