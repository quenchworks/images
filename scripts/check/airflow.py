#!/usr/bin/env python3
# airflow: PyPI apache-airflow. We ship the newest patch of each of the last 3 stable
# MINOR lines. Drop prereleases (rc/b/a/dev/post) — keep pure numeric X.Y.Z. depth=2
# groups candidates by major.minor so each of our shipped lines is compared to its
# newest patch (report_lines mirrors the recipe's per-minor-line discovery).
from _lib import pypi, report_lines
report_lines("airflow", pypi("apache-airflow"), 2,
             keep=lambda v: v.replace(".", "").isdigit())
