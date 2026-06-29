#!/usr/bin/env python3
# jenkins-inbound-agent: Wolfi apk 'jenkins-docker-agent' (FROM_SOURCE=0) — the agent
# script + agent.jar. Window = latest docker-agent versions in build.conf.
from _lib import wolfi, report
report("jenkins-inbound-agent", wolfi("jenkins-docker-agent"))
