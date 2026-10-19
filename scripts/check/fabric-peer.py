#!/usr/bin/env python3
# fabric-peer: GitHub releases of hyperledger/fabric (stable X.Y.Z), newest patch per minor
# line. We ship the 2.5 LTS and 3.1 lines; a newer line shows as NEW LINE.
import re
from _lib import github, report_lines
report_lines("fabric-peer", [v for v in github("hyperledger/fabric") if re.fullmatch(r"\d+\.\d+\.\d+", v)], 2)
