# Image update plan — 2026-08-31

From `uv run scripts/check-updates.py`:

```
summary: 229/229 apps checked · 65 updates · 0 errors · 0 blank · 0 unchecked
```

Checker coverage is **complete** -- every app has a `scripts/check/<slug>.py`, and the
tool now prints its own denominator, so an app added without one shows as `unchecked`
rather than vanishing from the count.

## Status legend

| status | meaning |
|---|---|
| **done** | bumped, gated 0-CVE on BOTH arches, committed |
| in progress | a build is running or queued on the shared lock |
| next | priority queue -- furthest behind, or a known CVE fix |
| pending | not started |
| blocked | cannot proceed; upstream-gated, reason recorded in build.conf |
| publish | no bump exists; only a publishing run fixes the published digest |

## Progress

- **done**: 7  (committed, both arches clean)
- **in progress**: 19  (bumped in-tree, awaiting a gate verdict)
- **next**: 18  (priority queue)
- **pending**: 44
- **blocked**: 2
- **publish-only**: 3

## The work

Every app with an available update, plus status. Multi-line apps need one gate per
release line.

| status | app | have | target(s) |
|---|---|---|---|
| in progress | elasticsearch | 9.5.1 | 9.5.2 |
| next | code-server | 4.132.0 | 4.135.0 |
| next | coolify-app | 4.3.4 | 4.3.15 |
| next | coroot | 1.23.3 | 1.25.0 |
| next | floci-full | 1.5.29 | 1.7.0 |
| next | forgejo | (per line) | 15.0.7 |
| next | ghost | 6.57.1 | 6.61.0 |
| next | gotify | 2.9.1 | 3.1.0 |
| next | grafana | 13.1.4 | 13.2.0 |
| next | kyverno | 1.18.1 | 1.19.0 |
| next | mediamtx | 1.19.3 | 1.20.1 |
| next | navidrome | 0.62.0 | 0.63.2 |
| next | ntfy | 2.25.0 | 2.28.0 |
| next | ollama | 0.32.13 | 0.33.2 |
| next | pyroscope | 2.1.0 | 2.3.0 |
| next | quickwit | 0.8.2 | 0.9.0 |
| next | renovate | 44.30.4 | 44.52.1 |
| next | tekton | 1.14.0 | 1.15.1 |
| next | woodpecker | 3.16.0 | 3.18.0 |
| pending | alertmanager | 0.33.1 | 0.34.0 |
| pending | argo-workflows | (per line) | 3.7.18 |
| pending | argocd | (per line) | 3.3.14, 3.4.8 |
| pending | centrifugo | 6.9.1 | 6.9.3 |
| pending | coolify-realtime | 1.0.17 | 1.0.18 |
| pending | crossplane | (per line) | 2.2.5, 2.3.5 |
| pending | documentdb | 0.114.0 | 0.116.0 |
| pending | dotnet | (per line) | 10.0.111 |
| pending | external-dns | 0.21.0 | 0.22.0 |
| pending | external-secrets | 2.9.0 | 2.10.0 |
| pending | filebrowser | 2.63.18 | 2.63.23, 2.63.22 |
| pending | floci | 1.6.0 | 1.7.0 |
| pending | headscale | 0.29.2 | 0.29.3 |
| pending | jaeger | 2.19.0 | 2.20.0 |
| pending | jdk | (per line) | 17.0.20.1, 21.0.12.1, 25.0.4.1 |
| pending | jre | (per line) | 17.0.20.1, 21.0.12.1, 25.0.4.1 |
| pending | k6 | 2.1.0 | 2.2.0 |
| pending | keda | 2.20.1 | 2.20.2 |
| pending | kube-state-metrics | 2.19.1 | 2.20.0 |
| pending | kuma | 2.14.0 | 2.14.3 |
| pending | livekit | 1.13.4 | 1.13.6 |
| pending | mariadb | (per line) | 11.4.13, 11.8.9, 12.3.3 |
| pending | miniflux | 2.3.2 | 2.3.3 |
| pending | node | (per line) | 24.20.0, 26.8.1 |
| pending | openfga | 1.18.1 | 1.19.0 |
| pending | perses | 0.53.1 | 0.54.0 |
| pending | pnpm | (per line) | 11.25.0 |
| pending | pocketbase | 0.39.5 | 0.40.1 |
| pending | postgrest | 16.1 | 16.2 |
| pending | pulumi | 3.257.0 | 3.260.0, 3.259.0, 3.258.0 |
| pending | redis-exporter | 1.89.0 | 1.90.0 |
| pending | rqlite | 10.2.5 | 10.2.7 |
| pending | scylladb | 2026.2.2 | 2026.2.5 |
| pending | spicedb | 1.56.0 | 1.56.1 |
| pending | syft | 1.51.0 | 1.51.1 |
| pending | tyk | 5.13.1 | 5.14.0 |
| pending | uv | 0.12.5 | 0.12.7, 0.12.6 |
| pending | vector | 0.57.0 | 0.58.0 |
| pending | victorialogs | 1.51.0 | 1.52.0 |
| pending | vllm | 0.27.1 | 0.28.0 |
| pending | weaviate | 1.39.1 | 1.39.2 |
| pending | xyops | 1.0.90 | 1.0.95 |
| pending | zitadel | 4.17.1 | 4.17.2 |
| pending | zot | 2.1.18 | 2.1.20 |
| blocked | coolify-helper | 1.0.15 | 1.0.16 |
| blocked | emqx | 6.2.2 | 6.2.3 |

### Already landed today (no longer in the list above)

| app | what |
|---|---|
| gitea | 1.27.3 |
| go | 1.26.7 |
| mongodb-exporter | 0.53.0 (new) |
| mysqld-exporter | 0.20.0 (new) |
| postgresql | uuid-ossp fix (18.6) |
| traefik | 3.7.12 |
| vikunja | 2.5.0 |

`keycloak` shows as **next** rather than done: 26.7.2 shipped today for the CRITICAL
CVE-2026-18963, and upstream has since released 26.7.3.

## In progress -- bumped in-tree, awaiting a gate verdict

These already have their new VERSIONS + sha256 in `build.conf`, which is why they no
longer appear as an UPDATE above. **None is committed**: a bump without a both-arch
`0 fixable CVEs` verdict is not done, and several are still queued behind the single
build lock.

| app | target(s) in-tree | gate |
|---|---|---|
| adminer | 5.5.1 6.0.0 6.0.1 | queued |
| atlantis | 0.47.1 | building now |
| buildkite-agent | 3.137.0 3.137.1 3.137.2 | queued |
| cadvisor | 0.60.5 | building now |
| clickhouse | 26.5.6.113 26.6.2.160 26.7.5.10 | queued |
| composer | 2.10.1 2.10.2 2.10.3 | building now |
| coredns | 1.14.7 | building now |
| cosmian-kms | 5.24.0 5.25.0 5.26.0 | queued |
| deno | 2.9.6 | queued |
| elasticsearch | 9.5.2 | queued |
| envoy-gateway | 1.8.3 1.9.1 | queued |
| gradle | 9.7.1 | queued |
| grype | 0.118.0 | building now |
| krakend | 2.13.10 | queued |
| loki | 3.7.7 | building now |
| mailpit | 1.31.0 | building now |
| nats | 2.14.4 2.14.5 2.14.6 | queued |
| opa | 1.20.1 | building now |
| poetry | 2.4.2 | queued |

`adminer` is in this set for a different reason: it carries an image FIX, not a
version bump -- `php-8.3-mysqlnd` was missing, so `mysqli` and `pdo_mysql` both
failed to load and a MySQL login answered "no supported PHP extensions available".
PostgreSQL and SQLite worked, so the image looked healthy and the 0-CVE gate passed.
## Publish-only -- no bump exists

Already newest upstream, recipes already gate clean. The CVEs are on the **published
digests**, never rebuilt. Bumping is impossible; only a publishing run fixes them.

| app | state |
|---|---|
| jenkins-inbound-agent | 27 fixable on the published digest |
| mimir | 16 fixable, already at 3.2.0 in-tree |
| sonar-scanner-cli | 12 fixable |

## Blocked

| app | why |
|---|---|
| coolify-helper | upstream-gated on docker/docker CVE-2026-33997; 1.0.15 rebuild failed at 98 fixable (96 fixable, 2 not) |
| emqx | 6.2.3 cannot be adopted yet -- see apps/emqx/build.conf |

## NEW LINE decisions -- 20 apps, human call

A new major/minor line is a catalog decision (support window, possibly new chart
values), not a patch bump. Deliberately NOT counted as updates. Decide per app.

| app | new line |
|---|---|
| argo-workflows | 4.1 (4.1.2) |
| argocd | 3.5 (3.5.2) |
| authentik | 2026.8 (2026.8.0) |
| crossplane | 2.4 (2.4.0) |
| forgejo | 16.0 (16.0.3) |
| go | go-1.27 (1.27.0) |
| grafana-alloy | 1.19 (1.19.2) |
| jdk | openjdk-26 (26.0.2.1) |
| jre | openjdk-26 (26.0.2.1) |
| mariadb | 13.1 (13.1.0) |
| matomo | 5.13 (5.13.0) |
| n8n | 2.36 (2.36.9) |
| n8n-runners | 2.36 (2.36.9) |
| neo4j | 2026.07 (2026.07.1) |
| openldap | openldap-2.7 (2.7.0) |
| pnpm | 12 (12.1.0) |
| ruby | ruby-4.0 (4.0.6) |
| sealed-secrets | 0.39 (0.39.1) |
| skywalking | 11.0 (11.0.0) |
| wordpress | 7.1 (7.1) |

## Cross-component pins

Not bumps of their own app -- pins that drifted from what a paired component expects.
Each needs a coordinated bump.

| what | state |
|---|---|
| contour 1.33.6 | wants envoy 1.38.3; we ship 1.38.1 -- same line, pairable, waits on Wolfi |
| kgateway 2.4.3 | same envoy 1.38.3 expectation |
| nginx-gateway-fabric 2.6.7 | pins nginx-agent 3.11.2; upstream 3.11.4 -- bump only with an NGF release |
| apisix-ingress-controller 2.2.0 | pins adc 0.27.1; api7/adc at 0.30.0 -- needs a controller bump too |
| kgateway-envoy | lockstep with apps/kgateway (2.3.7, 2.4.3) |
| linkerd-proxy | 2.366.0 paired correctly; 2.367.0 exists but must NOT move ahead of the control plane |

## Procedure

1. `apps/<app>/build.conf`: VERSIONS + per-version sha256. **Download and hash the real
   tarball.** Never invent or reuse a hash.
2. **Audit float pins BEFORE gating.** `go get M@V` pins EXACTLY, so a CVE floor left
   below what the new tag ships silently DOWNGRADES the module and can re-introduce the
   CVE it was added for. Use the `FLOOR`/`below_floor` pattern with the shipped-binary
   assertion (`docs/go-module-float.md` §2-3; apps/vikunja, apps/loki, apps/hydra).
3. Gate both arches, one build at a time:

   ```sh
   flock /tmp/qw-build.lock systemd-run --user --scope -p CPUQuota=300% -p MemoryMax=8G \
     env ARCHES=x86_64,aarch64 PUSH=0 bash ./scripts/build-image.sh <app> <version>
   ```

   `PUSH=0` is **not optional** -- `build-image.sh` does `PUSH="${PUSH:-1}"`, so an
   unqualified run publishes and signs. The version is **positional**; `VERSION=x` is
   ignored and gates the last VERSIONS entry, so gate EVERY version you ship.
4. Clean gate = `0 fixable CVEs (<arch>)` **once per arch**. No Trivy summary means a
   KILLED build -- a failure, never a pass.
5. Commit only both-arch-clean work. Explicit paths, never `git add -A`. Trailer:
   `Co-Authored-By: QuenchWorks <info@quench-works.com>`

One local build at a time (the box OOMs otherwise), so the target versions are the
wall-clock bottleneck. Hash-fetching, float audits and authoring parallelise; gating
does not.
