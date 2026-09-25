#!/usr/bin/env python3
# opentelemetry-operator: GitHub releases of open-telemetry/opentelemetry-operator (stable X.Y.Z), newest patch per minor line.
import re
from _lib import github, report_lines
report_lines("opentelemetry-operator", [v for v in github("open-telemetry/opentelemetry-operator") if re.fullmatch(r"\d+\.\d+\.\d+", v)], 2)
