#!/usr/bin/env python3
# concourse-git-resource: GitHub releases (concourse/git-resource). Latest line only.
from _lib import github, report
report("concourse-git-resource", github("concourse/git-resource"), n=1)
