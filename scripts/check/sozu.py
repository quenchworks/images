#!/usr/bin/env python3
# sozu: GitHub releases, tag X.Y.Z (no v prefix). Latest only, like the recipe.
from _lib import github, report
report("sozu", github("sozu-proxy/sozu"), n=1)
