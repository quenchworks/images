#!/usr/bin/env python3
# vitess: GitHub releases on vitessio/vitess, tags vX.Y.Z.
#
# Vitess versions by MAJOR, so a "line" here is the major and the window is the
# newest patch of each, checked at depth 1. That differs from most apps in this
# catalog, which line up on minor (depth 2).
#
# The recipe ships two lines, not the usual three: the 22 line cannot reach
# fixable=0 on any Go that Wolfi publishes. apps/vitess/build.conf carries the
# measurement and the re-add condition. So a report of a 22.x patch here is
# expected and is NOT actionable on its own.
from _lib import github, report_lines
report_lines("vitess", github("vitessio/vitess"), 1)
