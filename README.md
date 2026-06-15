# QuenchWorks images

The image factory. It builds hardened container images from source on [Wolfi](https://github.com/wolfi-dev) with [melange](https://github.com/chainguard-dev/melange) and [apko](https://github.com/chainguard-dev/apko), holds them to a hard 0-CVE gate, signs them with cosign, and publishes to GHCR.

**62 hardened images** for the infrastructure you actually run. No Dockerfiles. Nothing inherited from another distro. Free, signed, and rebuilt daily.

Part of [QuenchWorks](https://github.com/quenchworks), the 0-CVE replacement for the Bitnami catalog. Browse every image, with versions and digests, at [quench-works.com/images](https://quench-works.com/images).

## What ships here

Every image in the catalog:

- is **built from source**, no Dockerfile, nothing carried over from another distro. Where an upstream is infeasible to compile in CI (ClickHouse, ScyllaDB, CockroachDB, Dragonfly, MongoDB), we ship the project's own official binary and harden the base around it,
- clears a hard **0 fixable CVE** gate (Trivy, fail-on-fixable) before anything is published,
- runs as **nonroot (uid 1001)** on a **read-only root filesystem**,
- ships as a **multi-arch** index (linux/amd64 + linux/arm64), signed and pinned by digest,
- carries an **SBOM** and a **cosign** keyless signature.

The catalog spans databases, caches, search and vector, streaming, coordination, observability, gateways and proxies, object storage, secrets and identity, plus a container registry (Harbor), Git (Gitea), and CI/IaC (Atlantis).

## Pull and verify

```bash
# images are tagged by version (there is no :latest); swap redis:8.8.0 for any image and version
docker pull ghcr.io/quenchworks/images/redis:8.8.0

cosign verify ghcr.io/quenchworks/images/redis:8.8.0 \
  --certificate-identity-regexp 'https://github.com/quenchworks/.+' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## How a build runs

1. **melange** compiles the app from source into a signed APK.
2. **apko** assembles a minimal, nonroot, multi-arch image and writes it to a local tar.
3. **Trivy** scans that tar with `--exit-code 1 --ignore-unfixed`. A single fixable CVE fails the build, and nothing is published.
4. Only once the gate passes, apko publishes to `ghcr.io/quenchworks/images/<app>` as a multi-arch index.
5. **cosign** signs the digest (keyless).
6. A dispatch tells the [charts](https://github.com/quenchworks/charts) repo to repin the matching chart to the new digest.

The build runs on every change **and once a day**. That daily rebuild is the point: a clean scan stays true tomorrow instead of quietly aging out.

## Layout

```
catalog.yaml                 source of truth: app, version, source, license, tier, status
apps/<app>/melange.yaml      build the package from source
apps/<app>/apko.yaml         assemble the minimal nonroot image
apps/<app>/test.sh           smoke test the built image
.github/workflows/           per-app build, scan, sign, dispatch
```

## Add an app

Add a row to `catalog.yaml`, then create `apps/<app>/` with a melange build, an apko config, and a test. Charts are authored separately in the [charts](https://github.com/quenchworks/charts) repo, from each app's own upstream docs. See [CONTRIBUTING](https://github.com/quenchworks/.github/blob/main/CONTRIBUTING.md).

## A note on licensing

Most of the catalog is OSI-clean. Four datastores are source-available and carried with a loud license note in `catalog.yaml` and on the website, because they are **not** OSI-approved open source: MongoDB and Elasticsearch (SSPL-1.0), CockroachDB and Dragonfly (BUSL-1.1). Each names the clean alternative we recommend instead: Valkey, OpenSearch, FerretDB + DocumentDB.

## License

MIT for this repository's build configs and tooling. Each built image carries its upstream software's own license, recorded in `catalog.yaml` and the image labels.
