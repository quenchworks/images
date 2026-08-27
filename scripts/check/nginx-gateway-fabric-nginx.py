#!/usr/bin/env python3
# nginx-gateway-fabric-nginx (data plane): versioned by the NGF RELEASE, not by nginx.
# The control plane provisions this image with a tag equal to its own version, so the
# two apps move together and this checker watches the same source as the control plane.
#
# The image is built from three upstream trees, and only one of them is the version in
# build.conf, so the check also prints the component pins against upstream. Those are
# NOT free to bump on their own: each NGF release pins a specific trio in
# build/Dockerfile.nginx, and nginx-agent negotiates a protocol with the control plane.
# Treat a component drift line as "read the new release's Dockerfile.nginx", not as
# "bump this pin".
import re
from _lib import github, github_tags, report, current, BASE

report("nginx-gateway-fabric-nginx", github("nginx/nginx-gateway-fabric"))

ver = (current("nginx-gateway-fabric-nginx") or ["?"])[-1]
conf = open(f"{BASE}/apps/nginx-gateway-fabric-nginx/build.conf").read()


def pinned(name):
    m = re.search(rf"declare -A {name}=\(\s*\[{re.escape(ver)}\]=(\S+)", conf)
    return m.group(1) if m else "?"


def newest(cands):
    good = [c for c in cands if re.fullmatch(r"\d+\.\d+\.\d+", c)]
    return max(good, key=lambda v: tuple(int(n) for n in v.split(".")), default="?")


print("  components pinned by NGF %s (bump only with the release):" % ver)
for label, have, latest in (
    ("nginx", pinned("NGINX_VER"), newest([t.replace("release-", "") for t in github_tags("nginx/nginx")])),
    ("njs", pinned("NJS_VER"), newest(github_tags("nginx/njs"))),
    ("nginx-agent", pinned("AGENT_VER"), newest([t for t in github("nginx/agent") if t.startswith("3.")])),
):
    state = "ok" if have == latest else "upstream moved"
    print(f"    {label:<12s} have={have:>10s}  upstream-newest={latest:>10s}  {state}")
