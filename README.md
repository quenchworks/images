# Quenchworks images

The image factory. Builds hardened container images from source on
[Wolfi](https://github.com/wolfi-dev) with [melange](https://github.com/chainguard-dev/melange)
and [apko](https://github.com/chainguard-dev/apko), scans them with a hard 0-CVE gate, signs them
with cosign, and publishes to GHCR.

Part of [Quenchworks](https://github.com/quenchworks). See the org profile for the full picture.

## Layout

```
catalog.yaml                 source of truth: app, version, source, license, tier
apps/<app>/melange.yaml      build the package from source
apps/<app>/apko.yaml         assemble the minimal nonroot image
apps/<app>/test.sh           smoke test the built image
manifests/<app>.json         output: { repository, digest, builtAt } per app
.github/workflows/           per-app build, scan, sign, dispatch
```

## How a build runs

1. melange compiles the app from source into a signed APK.
2. apko assembles a minimal, nonroot, multi-arch image and writes it to a local tar.
3. Trivy scans that tar with `--exit-code 1 --ignore-unfixed`. A fixable CVE fails the build, and
   nothing is published.
4. Only after the gate passes, apko publishes to `ghcr.io/quenchworks/images/<app>`.
5. cosign signs the digest (keyless).
6. The digest is written to `manifests/<app>.json`, and a dispatch tells the charts repo to repin.

The build runs on change and once a day, so a clean scan stays true rather than aging out.

## Verify an image

```bash
cosign verify ghcr.io/quenchworks/images/redis \
  --certificate-identity-regexp 'https://github.com/quenchworks/.+' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Add an app

Add a row to `catalog.yaml`, then create `apps/<app>/` with a melange build, an apko config, and a
test. Charts are authored separately in the [charts](https://github.com/quenchworks/charts) repo,
from each app's own upstream docs. See [CONTRIBUTING](https://github.com/quenchworks/.github/blob/main/CONTRIBUTING.md).

## Two placeholders before the first build

- `apps/redis/melange.yaml`: set the real `expected-sha256` for the pinned source tarball.
- Repo secret: `CHARTS_DISPATCH_TOKEN` (lets the build notify the charts repo to repin).

## License

MIT for this repository's build configs and tooling. Each built image carries its upstream
software's own license, recorded in `catalog.yaml` and the image labels.
