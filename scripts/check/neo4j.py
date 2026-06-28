#!/usr/bin/env python3
# neo4j: build fetches dist.neo4j.org/neo4j-community-<ver>. CalVer YYYY.MM.P via
# GitHub tags (neo4j/neo4j), newest-first; per YYYY.MM line.
import re
from _lib import github_tags, report_lines
report_lines("neo4j", github_tags("neo4j/neo4j"), 2, keep=lambda t: bool(re.fullmatch(r"\d+\.\d+\.\d+", t)))
