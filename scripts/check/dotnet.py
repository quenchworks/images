#!/usr/bin/env python3
# dotnet: Wolfi dotnet-<major>-sdk (per major line 8/9/10).
from _lib import report_wolfi_suffix
report_wolfi_suffix("dotnet", r"^dotnet-\d+-sdk$", "dotnet-{maj}-sdk")
