# Longhorn on QuenchWorks images

QuenchWorks has no Longhorn chart. Install upstream's chart (`longhorn/longhorn` from
https://charts.longhorn.io) and point every image at ours. The images are built from source on
Wolfi for the 1.11, 1.12 and 1.13 release lines and keep upstream's layout: paths, users,
ENTRYPOINT and CMD. Upstream's chart and longhorn-manager can therefore run them unchanged.

| Upstream image | QuenchWorks image | Lines |
|---|---|---|
| longhornio/longhorn-manager | ghcr.io/quenchworks/images/longhorn-manager | 1.11.3, 1.12.1, 1.13.0 |
| longhornio/longhorn-engine | ghcr.io/quenchworks/images/longhorn-engine | 1.11.3, 1.12.1, 1.13.0 |
| longhornio/longhorn-instance-manager | ghcr.io/quenchworks/images/longhorn-instance-manager | 1.11.3, 1.12.1, 1.13.0 (v1 data engine only) |
| longhornio/longhorn-share-manager | ghcr.io/quenchworks/images/longhorn-share-manager | 1.11.3, 1.12.1, 1.13.0 |
| longhornio/backing-image-manager | ghcr.io/quenchworks/images/backing-image-manager | 1.11.3, 1.12.1, 1.13.0 |
| longhornio/longhorn-ui | ghcr.io/quenchworks/images/longhorn-ui | 1.11.3, 1.12.1, 1.13.0 |
| longhornio/support-bundle-kit | ghcr.io/quenchworks/images/support-bundle-kit | 0.0.88, 0.0.92, 0.0.98 |
| longhornio/csi-attacher | ghcr.io/quenchworks/images/csi-attacher | 4.11.0, 4.12.0, 4.13.0 |
| longhornio/csi-provisioner | ghcr.io/quenchworks/images/csi-provisioner | 6.1.2, 6.2.1, 6.3.0 |
| longhornio/csi-resizer | ghcr.io/quenchworks/images/csi-resizer | 2.1.0, 2.2.1, 2.3.0 |
| longhornio/csi-snapshotter | ghcr.io/quenchworks/images/csi-snapshotter | 8.4.0, 8.5.0, 8.6.0 |
| longhornio/csi-node-driver-registrar | ghcr.io/quenchworks/images/csi-node-driver-registrar | 2.16.0, 2.17.0, 2.18.0 |
| longhornio/livenessprobe | ghcr.io/quenchworks/images/csi-livenessprobe | 2.18.0, 2.19.0, 2.20.0 |

## Values for chart 1.13.0

Our tags have no `v` prefix. Set `registry` separately: the chart prefixes `docker.io` to a
repository when `registry` is empty.

```yaml
image:
  longhorn:
    engine: {registry: ghcr.io, repository: quenchworks/images/longhorn-engine, tag: "1.13.0"}
    manager: {registry: ghcr.io, repository: quenchworks/images/longhorn-manager, tag: "1.13.0"}
    ui: {registry: ghcr.io, repository: quenchworks/images/longhorn-ui, tag: "1.13.0"}
    instanceManager: {registry: ghcr.io, repository: quenchworks/images/longhorn-instance-manager, tag: "1.13.0"}
    shareManager: {registry: ghcr.io, repository: quenchworks/images/longhorn-share-manager, tag: "1.13.0"}
    backingImageManager: {registry: ghcr.io, repository: quenchworks/images/backing-image-manager, tag: "1.13.0"}
    supportBundleKit: {registry: ghcr.io, repository: quenchworks/images/support-bundle-kit, tag: "0.0.98"}
  csi:
    attacher: {registry: ghcr.io, repository: quenchworks/images/csi-attacher, tag: "4.13.0"}
    provisioner: {registry: ghcr.io, repository: quenchworks/images/csi-provisioner, tag: "6.3.0"}
    nodeDriverRegistrar: {registry: ghcr.io, repository: quenchworks/images/csi-node-driver-registrar, tag: "2.18.0"}
    resizer: {registry: ghcr.io, repository: quenchworks/images/csi-resizer, tag: "2.2.1"}
    snapshotter: {registry: ghcr.io, repository: quenchworks/images/csi-snapshotter, tag: "8.6.0"}
    livenessProbe: {registry: ghcr.io, repository: quenchworks/images/csi-livenessprobe, tag: "2.20.0"}
```

For the older charts, change the Longhorn tags to that line, and support-bundle-kit to 0.0.92
(1.12.1) or 0.0.88 (1.11.3). The sidecar pins in those charts, against what we build:

- **attacher 4.12.0, snapshotter 8.6.0, registrar 2.17.0, livenessprobe 2.19.0:** built
  exactly.
- **csi-resizer 2.2.0** (chart 1.11.3): use 2.2.1, the newest patch on that line.
- **csi-provisioner 5.3.0** (chart 1.12.1): not built; our lines start at 6.1. Keep upstream's
  provisioner image for 1.12.1. Chart 1.11.3 pins 6.3.0, which is built.

## Limits

- **V1 data engine only.** The instance-manager image does not contain SPDK, nvme-cli or
  go-spdk-helper, so enabling the v2 data engine fails at `start-spdk-tgt`.
- **Two npm packages in the UI bundle stay on vulnerable versions, on purpose.**
  - `decode-uri-component` 0.2.2: its only fixed release is ESM-only, and query-string 6
    `require()`s it.
  - The `@babel/runtime` beta under dva-core: stable 7.x removed the `core-js` paths it imports.
  - The scanner does not see either package (only the bundle ships). The reasons are recorded
    in `apps/longhorn-ui/melange.yaml`.

## What was verified, 2026-10-05

Upstream's 1.13.0 chart ran in kind with all 13 images above.

**Verified:**

- The manager, UI, driver deployer and CSI sidecars ran.
- The CSI plugin registered.
- A PVC was provisioned.
- The engine and replica processes ran under the instance-manager.
- tgtd served the iSCSI target, and discovery from the node succeeded.

**Not verified:** the iSCSI login and block device. In a kind node, iscsid cannot open the
kernel's iSCSI netlink socket outside the host's initial network namespace. A real node or VM
is needed for an end-to-end write test.

That kind run found two faults, both fixed:

- The manager image must have no ENTRYPOINT, because the CSI plugin passes arguments only.
- The instance-manager needs `/usr/local/bin/grpc_health_probe` at that exact path.
