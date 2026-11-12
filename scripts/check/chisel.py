#!/usr/bin/env python3
# chisel: GitHub releases, tag vX.Y.Z. Newest only, like the recipe.
from _lib import github, report
report("chisel", github("jpillora/chisel"), n=1)
