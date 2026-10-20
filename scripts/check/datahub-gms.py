#!/usr/bin/env python3
# datahub-gms: GitHub releases of datahub-project/datahub (vX.Y.Z.P, four parts), newest-only
# like the recipe; the image digest per arch is read from acryldata/datahub-gms:v<VER>.
import re
from _lib import github, report
report("datahub-gms", [v for v in github("datahub-project/datahub") if re.fullmatch(r"\d+\.\d+\.\d+(\.\d+)?", v)], n=1)
