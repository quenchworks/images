#!/usr/bin/env python3
# zipkin: GitHub releases of openzipkin/zipkin (stable X.Y.Z), newest-only like the recipe.
import re
from _lib import github, report
report("zipkin", [v for v in github("openzipkin/zipkin") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
