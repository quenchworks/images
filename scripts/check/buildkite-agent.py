#!/usr/bin/env python3
# buildkite-agent: GitHub releases (buildkite/agent).
from _lib import github, report
report("buildkite-agent", github("buildkite/agent"))
