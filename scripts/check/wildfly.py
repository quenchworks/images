#!/usr/bin/env python3
# wildfly: GitHub releases (wildfly/wildfly), tags X.Y.Z.Final. WildFly patches only
# its newest line, so we ship latest only; the image version drops ".Final".
import re
from _lib import github, report
cands = [t[:-len(".Final")] for t in github("wildfly/wildfly") if re.fullmatch(r"\d+\.\d+\.\d+\.Final", t)]
report("wildfly", cands, n=1)
