# Imagenes de QuenchWorks

[English](README.md) · [العربية](README.ar.md) · **Español**

<p align="center">
  <a href="https://quench-works.com/images"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/images.json" alt="images"></a>
  <a href="https://quench-works.com/charts"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/charts.json" alt="charts"></a>
  <a href="https://quench-works.com/security"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/cves.json" alt="open CVEs"></a>
  <a href="https://github.com/wolfi-dev"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/wolfi.json" alt="built from source"></a>
  <a href="https://docs.sigstore.dev/"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/cosign.json" alt="signed with cosign"></a>
  <a href="https://quench-works.com/images"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/multiarch.json" alt="multi-arch"></a>
  <a href="https://artifacthub.io/packages/search?org=quenchworks"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/artifacthub.json" alt="ArtifactHub"></a>
  <a href="https://github.com/quenchworks"><img src="https://img.shields.io/endpoint?url=https://quench-works.com/api/v1/badge/license.json" alt="license"></a>
</p>

La fabrica de imagenes. Construye imagenes de contenedor endurecidas desde el codigo fuente sobre [Wolfi](https://github.com/wolfi-dev) con [melange](https://github.com/chainguard-dev/melange) y [apko](https://github.com/chainguard-dev/apko), las somete a una barrera estricta de 0 CVE, las firma con cosign y las publica en GHCR.

<p align="center">
  <a href="https://quench-works.com"><img src="https://raw.githubusercontent.com/quenchworks/.github/main/profile/assets/demo.gif" alt="QuenchWorks en una terminal: ejecuta una imagen 0-CVE, verificala con cosign, despliega el chart de Helm y observa como el pod alcanza el estado Running." width="760"></a>
</p>

**90+ imagenes endurecidas** para la infraestructura que realmente ejecutas. Sin Dockerfiles. Nada heredado de otra distro. Gratis, firmadas y reconstruidas a diario.

Parte de [QuenchWorks](https://github.com/quenchworks), el reemplazo 0-CVE del catalogo de Bitnami. Explora cada imagen, con versiones y digests, en [quench-works.com/images](https://quench-works.com/images).

## Que se publica aqui

Cada imagen del catalogo:

- esta **construida desde el codigo fuente**, sin Dockerfile, sin nada arrastrado de otra distro. Cuando un upstream es inviable de compilar en CI (ClickHouse, ScyllaDB, CockroachDB, Dragonfly, MongoDB), publicamos el binario oficial del propio proyecto y endurecemos la base a su alrededor,
- supera una barrera estricta de **0 CVE corregibles** (Trivy, fail-on-fixable) antes de que se publique nada,
- se ejecuta como **nonroot (uid 1001)** sobre un **sistema de archivos raiz de solo lectura**,
- se publica como un indice **multi-arch** (linux/amd64 + linux/arm64), firmado y fijado por digest,
- lleva un **SBOM** y una firma keyless de **cosign**.

El catalogo abarca bases de datos, caches, busqueda y vectores, streaming, coordinacion, observabilidad, gateways y proxies, almacenamiento de objetos, secretos e identidad, ademas de un registro de contenedores (Harbor), Git (Gitea) y CI/IaC (Atlantis).

## Descarga y verifica

```bash
# images are tagged by version (there is no :latest); swap redis:8.8.0 for any image and version
docker pull ghcr.io/quenchworks/images/redis:8.8.0

cosign verify ghcr.io/quenchworks/images/redis:8.8.0 \
  --certificate-identity-regexp 'https://github.com/quenchworks/.+' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Como se ejecuta una construccion

1. **melange** compila la aplicacion desde el codigo fuente en un APK firmado.
2. **apko** ensambla una imagen minima, nonroot y multi-arch y la escribe en un tar local.
3. **Trivy** escanea ese tar con `--exit-code 1 --ignore-unfixed`. Un solo CVE corregible hace fallar la construccion, y no se publica nada.
4. Solo una vez que se supera la barrera, apko publica en `ghcr.io/quenchworks/images/<app>` como un indice multi-arch.
5. **cosign** firma el digest (keyless).
6. Un dispatch le indica al repositorio [charts](https://github.com/quenchworks/charts) que vuelva a fijar el chart correspondiente al nuevo digest.

La construccion se ejecuta en cada cambio **y una vez al dia**. Esa reconstruccion diaria es el objetivo: un escaneo limpio sigue siendo cierto manana en lugar de envejecer en silencio.

## Estructura

```
catalog.yaml                 source of truth: app, version, source, license, tier, status
apps/<app>/melange.yaml      build the package from source
apps/<app>/apko.yaml         assemble the minimal nonroot image
apps/<app>/test.sh           smoke test the built image
.github/workflows/           per-app build, scan, sign, dispatch
```

## Agrega una aplicacion

Agrega una fila a `catalog.yaml`, luego crea `apps/<app>/` con una construccion de melange, una configuracion de apko y una prueba. Los charts se redactan por separado en el repositorio [charts](https://github.com/quenchworks/charts), a partir de la documentacion upstream de cada aplicacion. Consulta [CONTRIBUTING](https://github.com/quenchworks/.github/blob/main/CONTRIBUTING.md).

## Una nota sobre las licencias

La mayor parte del catalogo es OSI-clean. Cuatro almacenes de datos son source-available y se incluyen con una nota de licencia destacada en `catalog.yaml` y en el sitio web, porque **no** son codigo abierto aprobado por OSI: MongoDB y Elasticsearch (SSPL-1.0), CockroachDB y Dragonfly (BUSL-1.1). Cada uno nombra la alternativa limpia que recomendamos en su lugar: Valkey, OpenSearch, FerretDB + DocumentDB.

## Licencia

MIT para las configuraciones de construccion y las herramientas de este repositorio. Cada imagen construida lleva la licencia propia de su software upstream, registrada en `catalog.yaml` y en las etiquetas de la imagen.
