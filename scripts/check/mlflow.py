#!/usr/bin/env python3
# mlflow: PyPI. Drop prereleases (rc/dev/bN) — keep pure X.Y.Z numeric versions.
from _lib import pypi, report
report("mlflow", pypi("mlflow"), keep=lambda v: v.replace(".", "").isdigit())
