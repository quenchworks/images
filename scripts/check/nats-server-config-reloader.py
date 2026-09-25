#!/usr/bin/env python3
# nats-server-config-reloader: GitHub releases of nats-io/nack (stable X.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("nats-server-config-reloader", [v for v in github("nats-io/nack") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
