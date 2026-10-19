#!/usr/bin/env python3
# selenium-node-firefox: GitHub releases of SeleniumHQ/selenium (tags selenium-X.Y.Z; the recipe fetches the
# selenium-server jar asset), newest two like the recipe.
import re
from _lib import github, report
report("selenium-node-firefox", [m[1] for t in github("SeleniumHQ/selenium") if (m := re.fullmatch(r"selenium-(\d+\.\d+\.\d+)", t))], n=2)
