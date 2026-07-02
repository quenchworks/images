#!/usr/bin/env python3
# vllm: PyPI. Drop prereleases (rc/dev/post/bN) — keep pure X.Y.Z numeric versions.
# NOTE: the image installs the CPU wheel (vllm==<VER>+cpu from wheels.vllm.ai/<VER>/cpu),
# but the version LINE is the same as PyPI's stable releases, so PyPI is the source of
# truth for have-vs-latest. Match the recipe's 3-line count.
from _lib import pypi, report
report("vllm", pypi("vllm"), keep=lambda v: v.replace(".", "").isdigit())
