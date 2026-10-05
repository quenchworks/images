#!/usr/bin/env python3
# emissary-apiext: GitHub releases of emissary-ingress/emissary, tag vX.Y.Z. Latest only for
# now (4.x runs stock Envoy, 3.x needs Emissary's Envoy fork).
from _lib import github, report
report("emissary-apiext", github("emissary-ingress/emissary"), n=1)
