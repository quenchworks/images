#!/usr/bin/env python3
# kratos: GitHub releases (ory/kratos). Ory unified versioning onto a shared
# CalVer-ish v25/v26 line; older v0.x/v1.x releases sort below it by vkey so
# they fall out of the top-n window automatically.
from _lib import github, report
report("kratos", github("ory/kratos"))
