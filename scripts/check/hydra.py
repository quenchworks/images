#!/usr/bin/env python3
# hydra: GitHub releases (ory/hydra). Ory unified versioning onto a shared
# CalVer-ish v25/v26 line; older v2.x releases sort below it by vkey so they
# fall out of the top-n window automatically.
from _lib import github, report
report("hydra", github("ory/hydra"))
