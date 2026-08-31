# Image update plan — 2026-08-31

Generated from `uv run scripts/check-updates.py`.

**49 apps behind, 63 target versions.** Plus two systemic gaps that matter more than
any single bump (sections 5 and 6).

## How to work this list

Every bump follows the same loop. None of it is optional:

1. Set `VERSIONS=(...)` and the per-version `sha256` in `apps/<app>/build.conf`.
   **Fetch and hash the real tarball.** Never invent, guess or reuse a hash.
2. **Audit the recipe's float pins before gating** (see section 4). This is the step
   most likely to be skipped and the most likely to cause a silent regression.
3. Gate locally, both arches, one build at a time:

   ```sh
   flock /tmp/qw-build.lock systemd-run --user --scope -p CPUQuota=300% -p MemoryMax=8G \
     env ARCHES=x86_64,aarch64 PUSH=0 bash ./scripts/build-image.sh <app> <version>
   ```

   `PUSH=0` is **not optional** — `build-image.sh` does `PUSH="${PUSH:-1}"`, so an
   unqualified run publishes and signs. The version is **positional**; `VERSION=x` is
   silently ignored and gates the wrong version.
4. A clean gate prints `0 fixable CVEs (<arch>)` **once per arch**. A log with no Trivy
   summary is a KILLED build — a failure, never a pass. Check `journalctl --user -n 50`.
5. Commit only what gated clean on both arches. Explicit paths, never `git add -A`,
   never stage `logs/`, `*.rendered.yaml`, `image.tar`, `packages/`, `melange.rsa*`,
   `sbom*`, `apko.build.log`. Trailer: `Co-Authored-By: QuenchWorks <info@quench-works.com>`.

A clean gate is a 0-CVE check only — `build-image.sh` does not run `test.sh`, so it is
**not** a boot test.

## 1. Publish-only — no version change exists

These are already on the newest upstream version and their recipes already gate clean.
The CVEs are on the **published digests**, which were never rebuilt. Bumping is
impossible; only a publishing run fixes them.

| app | fixable on published digest | note |
|---|---|---|
| jenkins-inbound-agent | 27 | recipe fixed 2026-08-27 (shadow git-lfs 3.7.1-r22) |
| sonar-scanner-cli | 12 | recipe moved to official SonarSource dist, logback 1.5.35 |
| mimir | 16 | already bumped to 3.2.0 in the tree, awaiting a build |

## 2. Float-pin audit required before bumping

These three have CVE floor pins that are now **below** what the newer upstream tag
ships. Because `go get M@V` sets an *exact* version, bumping without fixing the pin
silently **downgrades** the module and can re-introduce the very CVE the pin was for.

| app | bump | inverted pin |
|---|---|---|
| coredns | 1.14.6 -> 1.14.7 | grpc pin v1.82.1 vs tag v1.83.0 |
| opa | 1.19.1 -> 1.20.1 | otel pin v1.43.0 vs tag v1.44.0 |
| grype | 0.117.0 -> 0.118.0 | both of the above |

Fix by moving to the house `FLOOR`/`below_floor` pattern with the shipped-binary
assertion — see `docs/go-module-float.md` sections 2-3. Two apps were already
repaired this way (vikunja, loki); vikunja's inverted pins would have re-introduced
CVE-2025-69725 and CVE-2026-55677.

## 3. New MAJOR/MINOR lines available — a decision, not a bump

`check-updates.py` reports these separately because adopting a new line is a catalog
decision (new chart values, new support window), not a patch bump. Decide explicitly;
do not auto-adopt.

| app | new line |
|---|---|
| argocd | 3.5 (3.5.2) |
| crossplane | 2.4 (2.4.0) |
| jdk | openjdk-26 (26.0.2.1) |
| jre | openjdk-26 (26.0.2.1) |
| mariadb | 13.1 (13.1.0) |
| pnpm | 12 (12.1.0) |

## 4. The 49 apps, by shape

### Multi-line apps (one bump per release line)

| app | targets |
|---|---|
| argocd | 3.3.14, 3.4.8 |
| crossplane | 2.2.5, 2.3.5 |
| dotnet | 10.0.111 |
| jdk | 17.0.20.1, 21.0.12.1, 25.0.4.1 |
| jre | 17.0.20.1, 21.0.12.1, 25.0.4.1 |
| mariadb | 11.4.13, 11.8.9, 12.3.3 |
| node | 24.20.0, 26.8.1 |
| pnpm | 11.25.0 |

### Minor-version bumps — higher regression risk, gate carefully

| app | bump |
|---|---|
| alertmanager | 0.33.1 -> 0.34.0 (minor) |
| code-server | 4.132.0 -> 4.135.0 |
| coolify-app | 4.3.4 -> 4.3.15 (11 patches) |
| documentdb | 0.114.0 -> 0.116.0 |
| external-dns | 0.21.0 -> 0.22.0 (minor) |
| external-secrets | 2.9.0 -> 2.10.0 (minor) |
| floci | 1.6.0 -> 1.7.0 (minor) |
| ghost | 6.57.1 -> 6.61.0 (4 minors) |
| grafana | 13.1.4 -> 13.2.0 (minor) |
| kube-state-metrics | 2.19.1 -> 2.20.0 (minor) |
| ollama | 0.32.13 -> 0.33.2 (minor) |
| opa | 1.19.1 -> 1.20.1 (minor) |
| postgrest | 16.1 -> 16.2 |
| renovate | 44.30.4 -> 44.52.1 (22 minors behind) |
| vector | 0.57.0 -> 0.58.0 (minor) |
| vllm | 0.27.1 -> 0.28.0 (minor) |

### Patch bumps — mechanical

| app | have | targets |
|---|---|---|
| atlantis | 0.46.0 | 0.47.1 |
| buildkite-agent | 3.137.0 | 3.137.2, 3.137.1 |
| clickhouse | 26.7.3.19 | 26.7.5.10, 26.7.4.58 |
| composer | 2.10.2 | 2.10.3 |
| coolify-helper | 1.0.15 | 1.0.16 |
| coolify-realtime | 1.0.17 | 1.0.18 |
| coredns | 1.14.6 | 1.14.7 |
| cosmian-kms | 5.25.0 | 5.26.0 |
| deno | 2.9.5 | 2.9.6 |
| elasticsearch | 9.5.1 | 9.5.2 |
| emqx | 6.2.2 | 6.2.3 |
| envoy-gateway | 1.9.0 | 1.9.1 |
| gradle | 9.7.0 | 9.7.1 |
| grype | 0.117.0 | 0.118.0 |
| krakend | 2.13.9 | 2.13.10 |
| mailpit | 1.30.7 | 1.31.0 |
| nats | 2.14.5 | 2.14.6 |
| poetry | 2.4.1 | 2.4.2 |
| pulumi | 3.257.0 | 3.260.0, 3.259.0, 3.258.0 |
| redis-exporter | 1.89.0 | 1.90.0 |
| scylladb | 2026.2.2 | 2026.2.5 |
| syft | 1.51.0 | 1.51.1 |
| uv | 0.12.5 | 0.12.7, 0.12.6 |
| weaviate | 1.39.1 | 1.39.2 |
| xyops | 1.0.90 | 1.0.95 |

## 5. SYSTEMIC: 39 shipped apps have no version checker

`check-updates.py` globs `scripts/check/*.py`, so an app with **no checker file is
invisible** — it is never reported as behind, at any age. The survey's "190 apps" is
190 of ~229.

This is not hypothetical. **vikunja was one of these**, which is exactly how it sat two
releases behind while carrying 2 HIGH + 3 MEDIUM, without ever appearing as an UPDATE.

Apps with no checker:

- `argo-workflows`, `cadence`, `centrifugo`, `coroot`, `descheduler`, `filebrowser`
- `floci-full`, `forgejo`, `gitness`, `gotify`, `grafana-alloy`, `headscale`
- `jaeger`, `k6`, `keda`, `kuma`, `kyverno`, `livekit`
- `mediamtx`, `metrics-server`, `miniflux`, `navidrome`, `nsq`, `ntfy`
- `openfga`, `perses`, `pocketbase`, `pyroscope`, `quickwit`, `rqlite`
- `spicedb`, `step-ca`, `tekton`, `telegraf`, `tyk`, `velero`
- `victorialogs`, `woodpecker`, `zot`

Each needs a `scripts/check/<slug>.py` hitting that app's REAL upstream and encoding
the same version discovery the recipe uses. Reuse the `_lib` helpers (`github`,
`github_tags`, `npm`, `pypi`, `wolfi`, `scrape`, `json_get`); end with `report(...)`.
Match the recipe's version count (`n=3` default, `n=1` for ship-latest-only apps).

## 6. SYSTEMIC: broken checkers read as "up to date"

A checker that errors or returns zero candidates is indistinguishable from "nothing to
do" in the summary line. Same silent-failure shape as a missing checker.

- `linkerd-proxy` — **errors**: "not by matching this newest value directly"
- Any app reporting zero candidates should be treated as a broken check, not as current.

A checker that cannot fail loudly is worse than no checker, because it looks like coverage.

## 7. Suggested order

1. Section 1 (publishing) — highest security value, no build needed, main session owns it.
2. Section 2 (float audits) — do these before their bumps, or the bump regresses them.
3. Section 5 (missing checkers) — cheap, no build slot, and stops the next silent drift.
4. Patch bumps — mechanical, parallelisable across agents, but serialised on the build lock.
5. Minor bumps — one at a time, read the upstream changelog first.
6. Section 3 (new lines) — needs a human decision per app.

Only one local build can run at a time (the box OOMs otherwise), so the 63 target
versions are the wall-clock bottleneck regardless of how many agents work the list.
Authoring, hash-fetching, float audits and checker-writing all parallelise freely.
