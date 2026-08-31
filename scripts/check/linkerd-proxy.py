#!/usr/bin/env python3
# linkerd-proxy: linkerd/linkerd2-proxy is a SEPARATE repo from linkerd2, with
# its OWN tag convention (verified: tag_name values look like "release/v2.367.0",
# not bare "vX.Y.Z" and not the "edge-" of the linkerd2 monorepo).
#
# The version is NOT independently chosen, so this is NOT a "newest upstream tag"
# check. Each proxy pin MUST equal what the paired linkerd-control-plane edge
# release ships, or the data plane and control plane speak mismatched xDS/policy
# APIs. Upstream states that pairing in linkerd2's own `.proxy-version` file at
# each tag, so the discovery FOLLOWS THAT FILE at the exact edge tags
# apps/linkerd-control-plane/build.conf currently ships -- read per tag, never
# from the tip of main.
#
# This used to print a prose "go re-read .proxy-version yourself" note with no
# have=/latest= line, which check-updates.py counted as an ERROR: the app was
# effectively unchecked. It now resolves the pairing itself.
#
# Deliberately keyed to the control plane's CURRENT pins, not to upstream's
# newest edge release: the proxy must never move ahead of the control plane. When
# linkerd-control-plane's own checker reports an edge bump and that lands in its
# build.conf, this check surfaces the matching proxy version on the next run.
from _lib import current, github, scrape, vkey

RAW = "https://raw.githubusercontent.com/linkerd/linkerd2/edge-{}/.proxy-version"

cp = sorted(current("linkerd-control-plane"), key=vkey)
ours = sorted(current("linkerd-proxy"), key=vkey)

behind, errors = [], []
print("linkerd-proxy")
for i, cpv in enumerate(cp):
    try:
        want = scrape(RAW.format(cpv), r'v?(\d+\.\d+\.\d+)')[0]
    except Exception as e:
        want = None
        errors.append(f"{cpv}: {e}")
    have = ours[i] if i < len(ours) else "-"
    upd = bool(want) and want != have
    if upd:
        behind.append(want)
    print(f"  edge-{cpv}: have={have:>10s}  paired={(want or '?'):>10s}  "
          f"{'UPDATE' if upd else ('?' if not want else 'ok')}")

# Informational only: the newest proxy release upstream has cut. It becomes
# shippable when a control-plane edge release pins it, not before.
tags = [t[len("release/"):].lstrip("v")
        for t in github("linkerd/linkerd2-proxy") if t.startswith("release/")]
if tags:
    print(f"  newest upstream proxy release (not yet paired): {sorted(tags, key=vkey)[-1]}")

if errors:
    print(f"  => BROKEN CHECK -- could not read .proxy-version: {errors}")
elif len(ours) != len(cp):
    print(f"  => BROKEN CHECK -- {len(ours)} proxy pins vs {len(cp)} control-plane pins")
else:
    print(f"  => {'UPDATE -> ' + str(behind) if behind else 'ok'}")
