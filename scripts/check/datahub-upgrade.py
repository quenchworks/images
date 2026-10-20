#!/usr/bin/env python3
# datahub-upgrade: GitHub releases of datahub-project/datahub (vX.Y.Z.P, four parts), newest-only
# like the recipe; the image digest per arch is read from acryldata/datahub-upgrade:v<VER>.
import re
from _lib import github, report
report("datahub-upgrade", [v for v in github("datahub-project/datahub") if re.fullmatch(r"\d+\.\d+\.\d+(\.\d+)?", v)], n=1)
