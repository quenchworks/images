#!/usr/bin/env python3
# nginx-proxy-manager: GitHub releases of NginxProxyManager/nginx-proxy-manager (tags vX.Y.Z), newest-only.
import re
from _lib import github, report
report("nginx-proxy-manager", [v for v in github("NginxProxyManager/nginx-proxy-manager") if re.fullmatch(r"\d+\.\d+\.\d+", v)], n=1)
