#!/usr/bin/env python3
# mattermost: build fetches releases.mattermost.com/<ver>/mattermost-team-...,
# but versions track GitHub releases (mattermost/mattermost), tag vX.Y.Z.
from _lib import github, report
report("mattermost", github("mattermost/mattermost"))
