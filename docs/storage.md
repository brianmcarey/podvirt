# Storage and Cache Behavior

This document explains how `podvirt` handles local disk images, container disks, and the cache directories under `~/.cache/podvirt/`.

## Local disk images

A local disk is referenced with:

```yaml
disks:
  - name: rootdisk
    source:
      image: /path/to/disk.qcow2
```

At `podvirt create` time, the path must already exist on the host.

`podvirt` mounts the disk into the `virt-launcher` container at the path expected by KubeVirt's standalone disk layout.

If `disk.size` / `--disk-size` is set, `podvirt` first creates a self-contained qcow2 copy under the cache, resizes that copy, and then mounts the resized copy instead of the original file. The source image is never modified in place.

## Container disks

A container disk is referenced with:

```yaml
disks:
  - name: rootdisk
    source:
      containerImage: quay.io/containerdisks/fedora:43
```

During `podvirt create`:

1. the OCI image is pulled with Podman
2. a temporary container is created
3. `/disk/` is copied out into the local cache
4. the config is rewritten internally to use the extracted host-side image path

After extraction, the rest of the VM flow treats the disk like a normal local image.

## Cache layout

The project uses `~/.cache/podvirt/` for several runtime artifacts:

- `containerdisks/` — extracted container disk images
- `resized-disks/` — per-VM resized qcow2 copies created from source images
- `qemu-caps/` — cached QEMU capability probes
- `qemu-support/` — generated helper/wrapper assets
- `libvirt-logs/` — extra logs preserved when `PODVIRT_DEBUG=1`

These paths are safe to delete; they will be recreated as needed.

## Cache reuse

Container disk extraction is cached by image reference. Re-running `podvirt create` with the same container image will normally reuse the extracted disk instead of pulling and copying it again.

This speeds up repeated VM creation substantially.

## Cleaning cache

Use:

```bash
podvirt clean-cache
```

or:

```bash
podvirt clean-cache --force
```

This only removes cached `podvirt` artifacts. It does **not** remove:

- user-supplied local disk files
- existing VM containers
- images in Podman's own storage

It does remove extracted container disks and resized disk copies, so the next `podvirt create` may need to recreate them.

## Format handling

`podvirt` supports both raw and qcow2 local disk usage. For extracted container disks, it detects qcow2 content and preserves that information so the QEMU wrapper can configure the guest launch correctly.

If extracted-disk format handling fails, `podvirt create` now returns an explicit error instead of silently continuing with a potentially incorrect path.

## Recommendations

- Use qcow2 cloud images for convenience and smaller download size.
- Keep large custom VM images outside the cache tree and reference them with `source.image`.
- Use `clean-cache` if you want to reclaim extracted container disk space, resized disk copies, or refresh cached helper state.
