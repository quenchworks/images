# Image update plan — 2026-08-31

Regenerated from `uv run scripts/check-updates.py` after the checker-coverage fix.

```
summary: 229/229 apps checked · 65 updates · 0 errors · 0 blank · 0 unchecked
```

**Checker coverage is now complete.** The previous run read `190 apps` and silently
omitted 39; those 39 are now covered and 30 of them turned out to be behind. The tool
also reports its own denominator, so a future app added without a checker shows up as
`unchecked` instead of vanishing. `linkerd-proxy`'s broken checker is fixed and now
verifies the proxy against the `.proxy-version` its paired control-plane edge ships.

**65 apps behind, 78 target versions.** The queue grew from 49 because the previously
invisible apps are finally counted -- that growth is the fix working, not a regression.

## How to work this list

1. `apps/<app>/build.conf`: set VERSIONS and the per-version sha256. **Download the
   real tarball and hash it.** Never invent, guess or reuse a hash. Cross-check against
   upstream's published `.sha256` where one exists.
2. **Audit float pins BEFORE gating.** `go get M@V` pins EXACTLY, so a CVE floor left
   below what the new tag ships silently DOWNGRADES the module and can re-introduce the
   CVE the pin was added for. Convert to the `FLOOR`/`below_floor` pattern with the
   shipped-binary assertion (`docs/go-module-float.md` §2-3; see apps/vikunja,
   apps/loki, apps/hydra, apps/telegraf).
3. Gate locally, both arches, one build at a time:

   ```sh
   flock /tmp/qw-build.lock systemd-run --user --scope -p CPUQuota=300% -p MemoryMax=8G \
     env ARCHES=x86_64,aarch64 PUSH=0 bash ./scripts/build-image.sh <app> <version>
   ```

   `PUSH=0` is **not optional** -- `build-image.sh` does `PUSH="${PUSH:-1}"`, so an
   unqualified run publishes and signs. The version is **positional**; `VERSION=x` is
   silently ignored and gates the last VERSIONS entry, so gate EVERY version you ship.
4. A clean gate prints `0 fixable CVEs (<arch>)` **once per arch**. A log with no Trivy
   summary is a KILLED build -- a failure, never a pass (`journalctl --user -n 50` for
   OOM). Launch detached; long builds outlive a foreground call.
5. Commit only what gated clean on BOTH arches. Explicit paths, never `git add -A`,
   never stage `logs/`, `*.rendered.yaml`, `image.tar`, `packages/`, `melange.rsa*`,
   `sbom*`, `apko.build.log`. Trailer:
   `Co-Authored-By: QuenchWorks <info@quench-works.com>`

A clean gate is a 0-CVE check only -- `build-image.sh` does not run `test.sh`, so it is
**not** a boot test.

## 1. Publish-only -- no version bump exists

Already newest upstream; recipes already gate clean. The CVEs are on the **published
digests**, which were never rebuilt. Only a publishing run fixes these.

| app | fixable on published digest | note |
|---|---|---|
| jenkins-inbound-agent | 27 | recipe fixed 2026-08-27 (shadow git-lfs 3.7.1-r22) |
| sonar-scanner-cli | 12 | recipe on official SonarSource dist, logback 1.5.35 |
| mimir | 16 | already at 3.2.0 in-tree, awaiting a build |
| coolify-helper | 282 | BLOCKED=1 -- upstream-gated on docker/docker, see below |

`coolify-helper` is the exception: its 1.0.15 rebuild was attempted and **failed the
gate at 98 fixable**. 96 of those are fixable, but 2 are upstream-gated on
`docker/docker` CVE-2026-33997, whose fix (29.3.1) does not exist as a Go module on any
path -- and buildx still imports `docker/pkg/namesgenerator`. It stays `BLOCKED=1`.
A 1.0.16 bump is available and worth retrying, but expect the same 2 rows.

## 2. In progress right now

Do not pick these up; agents own them.

- **float-pin fixes + bumps**: coredns, grype, opa (all three had inverted pins)
- **patch batch A**: atlantis, buildkite-agent, clickhouse, composer, cosmian-kms, deno, elasticsearch, emqx, envoy-gateway, gradle, krakend, mailpit, nats, poetry

Landed earlier today: gitea, go, keycloak, loki, mongodb-exporter, mysqld-exporter, traefik, vikunja.

## 3. Furthest behind -- do these first

Most upstream releases skipped, so most accumulated fixes and most regression risk.
Read the changelog before gating.

| app | bump |
|---|---|
| code-server | 4.132.0 -> 4.135.0 |
| coolify-app | 4.3.4 -> 4.3.15 |
| coroot | 1.23.3 -> 1.25.0 |
| floci-full | 1.5.29 -> 1.7.0 |
| forgejo | 15.0.3 -> 15.0.7 |
| ghost | 6.57.1 -> 6.61.0 |
| gotify | 2.9.1 -> 3.1.0 |
| grafana | 13.1.4 -> 13.2.0 |
| kyverno | 1.18.1 -> 1.19.0 |
| mediamtx | 1.19.3 -> 1.20.1 |
| navidrome | 0.62.0 -> 0.63.2 |
| ntfy | 2.25.0 -> 2.28.0 |
| ollama | 0.32.13 -> 0.33.2 |
| pyroscope | 2.1.0 -> 2.3.0 |
| quickwit | 0.8.2 -> 0.9.0 |
| renovate | 44.30.4 -> 44.52.1 |
| tekton | 1.14.0 -> 1.15.1 |
| woodpecker | 3.16.0 -> 3.18.0 |

## 4. Multi-line apps -- one bump per release line

| app | targets |
|---|---|
| argo-workflows | 3.7.18 |
| argocd | 3.3.14, 3.4.8 |
| crossplane | 2.2.5, 2.3.5 |
| dotnet | 10.0.111 |
| forgejo | 15.0.7 |
| jdk | 17.0.20.1, 21.0.12.1, 25.0.4.1 |
| jre | 17.0.20.1, 21.0.12.1, 25.0.4.1 |
| mariadb | 11.4.13, 11.8.9, 12.3.3 |
| node | 24.20.0, 26.8.1 |
| pnpm | 11.25.0 |

## 5. Remaining single-version bumps

| app | have | target |
|---|---|---|
| alertmanager | 0.33.1 | 0.34.0 |
| centrifugo | 6.9.1 | 6.9.3 |
| coolify-helper | 1.0.15 | 1.0.16 |
| coolify-realtime | 1.0.17 | 1.0.18 |
| documentdb | 0.114.0 | 0.116.0 |
| external-dns | 0.21.0 | 0.22.0 |
| external-secrets | 2.9.0 | 2.10.0 |
| filebrowser | 2.63.18 | 2.63.23, 2.63.22 |
| floci | 1.6.0 | 1.7.0 |
| headscale | 0.29.2 | 0.29.3 |
| jaeger | 2.19.0 | 2.20.0 |
| k6 | 2.1.0 | 2.2.0 |
| keda | 2.20.1 | 2.20.2 |
| kube-state-metrics | 2.19.1 | 2.20.0 |
| kuma | 2.14.0 | 2.14.3 |
| livekit | 1.13.4 | 1.13.6 |
| miniflux | 2.3.2 | 2.3.3 |
| openfga | 1.18.1 | 1.19.0 |
| perses | 0.53.1 | 0.54.0 |
| pocketbase | 0.39.5 | 0.40.1 |
| postgrest | 16.1 | 16.2 |
| pulumi | 3.257.0 | 3.260.0, 3.259.0, 3.258.0 |
| redis-exporter | 1.89.0 | 1.90.0 |
| rqlite | 10.2.5 | 10.2.7 |
| scylladb | 2026.2.2 | 2026.2.5 |
| spicedb | 1.56.0 | 1.56.1 |
| syft | 1.51.0 | 1.51.1 |
| tyk | 5.13.1 | 5.14.0 |
| uv | 0.12.5 | 0.12.7, 0.12.6 |
| vector | 0.57.0 | 0.58.0 |
| victorialogs | 1.51.0 | 1.52.0 |
| vllm | 0.27.1 | 0.28.0 |
| weaviate | 1.39.1 | 1.39.2 |
| xyops | 1.0.90 | 1.0.95 |
| zitadel | 4.17.1 | 4.17.2 |
| zot | 2.1.18 | 2.1.20 |

## 6. NEW LINE decisions -- 20 apps, human call required

A new major/minor line is a catalog decision (new support window, possibly new chart
values), not a patch bump. `check-updates.py` reports these separately and does NOT
count them as updates. Decide per app; do not auto-adopt.

| app | new line available |
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

## 7. Cross-component pins the checkers flagged

These are not version bumps of their own app -- they are pins that have drifted from
what a paired component expects. Each needs a coordinated bump, not an independent one.

| what | state |
|---|---|
| contour 1.33.6 | wants envoy 1.38.3; we ship 1.38.1 (same line, ours older) -- bump envoy when Wolfi has 1.38.3 |
| kgateway 2.4.3 | same envoy 1.38.3 expectation |
| nginx-gateway-fabric 2.6.7 | pins nginx-agent 3.11.2; upstream is at 3.11.4 -- bump only with an NGF release |
| apisix-ingress-controller 2.2.0 | pins adc 0.27.1; api7/adc is at 0.30.0 -- needs a controller bump too |
| kgateway-envoy | lockstep with apps/kgateway (2.3.7, 2.4.3) -- keep in sync |
| linkerd-proxy | 2.366.0 paired correctly; upstream 2.367.0 exists but must NOT move ahead of the control plane |

## 8. Order of work

1. **Section 1** -- publishing. Highest security value per unit of effort, needs no
   build slot, and it is the only thing that fixes an already-published digest.
2. **Section 3** -- furthest behind, since they carry the most unshipped fixes.
3. **Sections 4-5** -- mechanical; parallelise the authoring, but the gates serialise.
4. **Section 7** -- coordinated pins, needs a paired decision.
5. **Section 6** -- new lines, one human decision each.

Only ONE local build runs at a time (the box OOMs otherwise), so the 78 target versions
are the wall-clock bottleneck no matter how many agents work the list. Hash-fetching,
float audits and recipe authoring all parallelise freely; gating does not.
