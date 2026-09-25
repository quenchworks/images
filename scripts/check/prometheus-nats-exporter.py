#!/usr/bin/env python3
# prometheus-nats-exporter: GitHub releases of nats-io/prometheus-nats-exporter (stable X.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("prometheus-nats-exporter", [v for v in github("nats-io/prometheus-nats-exporter") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
