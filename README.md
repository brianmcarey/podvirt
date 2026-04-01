# podvirt

`podvirt` wraps KubeVirt's `virt-launcher` container image with Podman, giving you a simple CLI to create, start, stop, and manage VMs using the same underlying technology as KubeVirt. This is useful on Atomic distros like Fedora Silverblue where you might not have the required packages layered to quickly spin up virtual machines.

## Features

- Create and run VMs from a simple YAML config or CLI flags
- Uses KubeVirt's `virt-launcher`
- Rootless operation via Podman (user only needs `/dev/kvm` access)
- Full VM lifecycle: `create`, `start`, `stop`, `list`, `status`, `delete`, `console`, `ssh`, `clean-cache`
- SSH access via cloud-init key injection and auto-detected port forwarding
- Serial console access (attach to `virsh console` via `podman exec`)- Table, JSON, and YAML output formats

## System Requirements

| Component | Requirement |
|-----------|-------------|
| Podman    | v5.0+ |
| KVM       | `/dev/kvm` must exist; user must be in `kvm` group |


## Installation

### Build from source

```bash
git clone https://github.com/brianmcarey/podvirt
cd podvirt
make build
sudo install -m 755 bin/podvirt /usr/local/bin/podvirt
```

### Install from a release

Tagged releases are intended to include:

- Linux `x86_64` and `aarch64` tarballs
- raw Linux binaries
- RPM packages

If you are installing manually from a release tarball:

```bash
tar -xzf podvirt_<version>_linux_x86_64.tar.gz
sudo install -m 755 podvirt /usr/local/bin/podvirt
```

## Quick Start

### 1. Enable the Podman socket

```bash
systemctl --user enable --now podman.socket
```

### 2. Ensure KVM access

```bash
sudo usermod -aG kvm $USER
# Log out and back in for group membership to take effect
```

### 3. Create a VM with SSH access

```bash
podvirt create --name myvm --cpus 2 --memory 2Gi \
  --image quay.io/containerdisks/fedora:43 \
  --ssh-key ~/.ssh/ssh-key.pub \
  --user fedora \
  --port 2222:22
podvirt start myvm
podvirt ssh myvm
```

### 4. Create a VM from a config file

```bash
podvirt create --config examples/fedora-vm.yaml
podvirt start fedora-vm
```

### 5. Connect to the console

```bash
podvirt console fedora-vm              # serial by default
podvirt console fedora-vm --type vnc
podvirt console fedora-vm --type serial
```

### 6. Stop and delete

```bash
podvirt stop fedora-vm
podvirt delete fedora-vm
```

## Commands

| Command                       | Description |
|-------------------------------|-------------|
| `podvirt create`              | Create a VM (not started) |
| `podvirt start <name>`        | Start a VM |
| `podvirt stop <name>`         | Stop a VM |
| `podvirt list`                | List all VMs |
| `podvirt status <name>`       | Show detailed VM status |
| `podvirt delete <name>`       | Delete a VM |
| `podvirt console <name>`      | Connect to VM console |
| `podvirt ssh <name>`          | SSH into a running VM |
| `podvirt clean-cache`         | Remove cached podvirt data |

Run `podvirt <command> --help` for full flag documentation.

## Configuration

VMs are described in a simple YAML format:

```yaml
apiVersion: podvirt.io/v1alpha1
kind: VirtualMachine
metadata:
  name: my-vm
spec:
  cpu:
    cores: 2
  memory: 2Gi
  disks:
    - name: rootdisk
      source:
        image: /var/lib/podvirt/images/fedora-43.qcow2
      bus: virtio
  networks:
    - name: default
      type: masquerade
      portForwards:
        - hostPort: 2222
          vmPort: 22
  cloudInit:
    user: fedora
    sshKeys:
      - ssh-ed25519 AAAA... user@host
  console:
    type: vnc
    port: 5900
```

See [docs/configuration.md](docs/configuration.md) for the full schema reference and CLI flag documentation.

Example configs are in [examples/](examples/).

## How It Works

```
podvirt CLI
    │
    ├─► Podman API  ──► virt-launcher container
    │                       │
    │                       ├── libvirt (internal, no host daemon)
    │                       └── QEMU/KVM virtual machine
    │
    └─► podman exec virsh  ──► query domain state
```

Each VM runs as a Podman container using the `quay.io/kubevirt/virt-launcher` image. The virt-launcher process manages `libvirt` and `QEMU` internally, so no host-level daemons are needed.

The VM specification is passed via the `STANDALONE_VMI` environment variable as a JSON-encoded KubeVirt `VirtualMachineInstance` object.

## Disk Images

Two disk source types are supported:

**Local file** (qcow2 or raw):
```yaml
source:
  image: /path/to/disk.qcow2
```

**Container disk** (OCI image from a registry):
```yaml
source:
  containerImage: quay.io/containerdisks/fedora:43
```

Container disks are pulled automatically. See [quay.io/containerdisks](https://quay.io/organization/containerdisks) for available images.

For cache behavior, extraction details, and cleanup guidance, see [docs/storage.md](docs/storage.md).

## virt-launcher Image

The default virt-launcher image is:
```
quay.io/kubevirt/virt-launcher:v1.7.0
```

Override it per-VM with the `--launcher-image` flag:
```bash
podvirt create --config vm.yaml --launcher-image quay.io/kubevirt/virt-launcher:v1.8.0
```

## Debugging

Set `PODVIRT_DEBUG=1` to preserve extra runtime diagnostics:

```bash
PODVIRT_DEBUG=1 podvirt create --name debugvm --image quay.io/containerdisks/fedora:43
```

When debug mode is enabled, additional logs may be written under `~/.cache/podvirt/libvirt-logs/`.

## License

Apache 2.0 — see [LICENSE](LICENSE).
