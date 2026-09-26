#!/usr/bin/env python3
# openresty: releases on GitHub (openresty/openresty), tags vX.Y.Z.N (github_tags
# strips the v); rc tags are skipped. We ship the newest release of the last two X.Y lines (1.29, 1.31);
# report_lines compares per line and flags any newer line.
import re
from _lib import github_tags, report_lines
tags = [t for t in github_tags("openresty/openresty") if re.fullmatch(r"\d+\.\d+\.\d+\.\d+", t)]
report_lines("openresty", tags, depth=2)
