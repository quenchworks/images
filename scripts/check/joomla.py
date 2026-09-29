#!/usr/bin/env python3
# joomla: GitHub releases of joomla/joomla-cms, the same place the recipe fetches its
# Full_Package archive from. We ship 5.4 (long-term support) and 6.1 (current), so
# report_lines compares per X.Y line; a line older than our newest is never reported new.
from _lib import github, report_lines
report_lines("joomla", github("joomla/joomla-cms"), depth=2)
