#!/usr/bin/env python3
# linkerd-proxy: linkerd/linkerd2-proxy is a SEPARATE repo from linkerd2, with
# its OWN tag convention -- verified, not assumed: `gh api
# repos/linkerd/linkerd2-proxy/releases` shows tag_name values like
# "release/v2.367.0", not bare "vX.Y.Z" and not "stable-"/"edge-" like the
# linkerd2 monorepo. github()'s lstrip("v") only strips a leading "v", so the
# "release/" prefix is stripped here before that.
#
# NOT a per-minor-line check like istiod/linkerd-control-plane: this repo is a
# linear release train (each release bumps the middle number; there is no
# recurring X.Y with multiple patches to group by), and more importantly this
# app's versions are NOT independently chosen at all -- each one must match
# whatever linkerd-control-plane's corresponding edge release actually pins in
# its own `.proxy-version` file (see linkerd-proxy/build.conf). So this script
# only reports the newest release upstream has published, as a prompt to go
# re-read `.proxy-version` at each linkerd-control-plane tag this factory
# tracks -- it deliberately does NOT flag our three older pins as "UPDATE",
# because bumping them to the single newest proxy release would desync them
# from the control-plane versions they're paired with.
from _lib import github, current, vkey

cur = current("linkerd-proxy")
tags = [t[len("release/"):].lstrip("v") for t in github("linkerd/linkerd2-proxy") if t.startswith("release/")]
newest = sorted(tags, key=vkey, reverse=True)[0]

print("linkerd-proxy")
for c in cur:
    print(f"  pinned: {c:>10s}  (paired to a linkerd-control-plane release, not independently upgraded)")
print(f"  newest upstream release: {newest}")
print("  => re-check each pin via .proxy-version at the linkerd-control-plane tag it's paired with,")
print("     not by matching this newest value directly")
