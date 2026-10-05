#!/usr/bin/env python3
# terragrunt: GitHub releases (gruntwork-io/terragrunt). Newest line only (see build.conf).
from _lib import github, report
report("terragrunt", github("gruntwork-io/terragrunt"), n=1)
