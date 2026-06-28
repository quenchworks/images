#!/usr/bin/env python3
# dotnet-runtime: Wolfi dotnet-<major>-runtime.
from _lib import report_wolfi_suffix
report_wolfi_suffix("dotnet-runtime", r"^dotnet-\d+-runtime$", "dotnet-{maj}-runtime")
