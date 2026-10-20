#!/usr/bin/env python3
# mattermost: build fetches releases.mattermost.com/<ver>/mattermost-team-...,
# but versions track GitHub releases (mattermost/mattermost), tag vX.Y.Z.
# VERSIONS holds the chart's line (11.11 as of 2026-10-20) plus the newest, so an
# 11.11.x patch listed next to have=12.x is a real update for the chart's line.
# Lines we do not ship (the 11.7 ESR, 11.10) are not reported.
from _lib import github, report
report("mattermost", github("mattermost/mattermost"))
