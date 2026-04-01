# Configuration Reference

`podvirt` VMs are described in a YAML file or via CLI flags. This document covers the full schema, all flags, and how they interact.

## YAML Config File

Pass a config file to any command with `--config`:

```bash
podvirt create --config vm.yaml
```

### Top-level structure

```yaml
apiVersion: podvirt.io/v1alpha1   # required; must be this value
kind: VirtualMachine              # required; must be this value
metadata:
  name: <string>                  # required; DNS label (a-z, 0-9, -)
  labels:                         # optional; arbitrary key/value pairs
    key: value
spec:
  cpu: ...
  memory: <string>
  disks: [...]
  networks: [...]
  boot: ...       # optional
  console: ...    # optional
```

---

### `spec.cpu`

| Field     | Type   | Default | Description |
|-----------|--------|---------|-------------|
| `cores`   | int    | `1`     | Number of virtual CPU cores |
| `sockets` | int    | `1`     | Number of CPU sockets |
| `threads` | int    | `1`     | Threads per core (for SMT/hyperthreading) |

**Example:**
```yaml
cpu:
  cores: 4
  sockets: 1
  threads: 2   # 8 logical CPUs total (4 cores × 2 threads)
```

---

### `spec.memory`

A memory size string using binary suffixes: `Mi` (mebibytes) or `Gi` (gibibytes).

**Examples:** `512Mi`, `1Gi`, `4Gi`

**Minimum:** `128Mi`  
**Default (CLI):** `1Gi`

---

### `spec.disks`

A list of disk devices attached to the VM. At least one disk is required.

| Field          | Type   | Default   | Description |
|----------------|--------|-----------|-------------|
| `name`         | string | required  | Unique name for this disk |
| `source.image` | string | —         | Path to a local disk image (qcow2 or raw) |
| `source.containerImage` | string | — | OCI image URL (container disk) |
| `size`         | string | —         | Optional desired virtual disk size, e.g. `20Gi` |
| `bus`          | string | `virtio`  | Disk bus type: `virtio`, `sata`, `scsi` |

Exactly one of `source.image` or `source.containerImage` must be set per disk.

**Local disk example:**
```yaml
disks:
  - name: rootdisk
    source:
      image: /var/lib/podvirt/images/fedora-41.qcow2
    size: 20Gi
    bus: virtio
```

**Container disk example:**
```yaml
disks:
  - name: rootdisk
    source:
      containerImage: quay.io/containerdisks/fedora:43
    bus: virtio
```

Container disk images are pulled automatically. Browse available images at [quay.io/containerdisks](https://quay.io/organization/containerdisks).

When `size` is set, `podvirt create` makes a per-VM qcow2 copy in the cache, resizes that copy, and leaves the original source image untouched.

---

### `spec.networks`

A list of network interfaces. If omitted, `podvirt` adds a default `masquerade` network named `default`.

| Field    | Type   | Default      | Description |
|----------|--------|--------------|-------------|
| `name`   | string | required     | Interface name |
| `type`   | string | `masquerade` | Network type. Only `masquerade` is currently supported. |
| `portForwards` | []object | — | Optional host-to-VM port mappings for this network |

**`masquerade`** — standalone `podvirt` uses its current user-mode networking path for guest connectivity, with host port mappings carried through from this network config.

**Example:**
```yaml
networks:
  - name: default
    type: masquerade
```

---

### `spec.networks[].portForwards` (optional)

A list of host→VM port mappings. Required to reach the VM via SSH or any other TCP service from the host.

| Field        | Type   | Default | Description |
|--------------|--------|---------|-------------|
| `hostPort`   | int    | required | Port on the host |
| `vmPort`     | int    | required | Port inside the VM |
| `protocol`   | string | `tcp`   | Protocol: `tcp` or `udp` |

**Example:**
```yaml
networks:
  - name: default
    type: masquerade
    portForwards:
      - hostPort: 2222
        vmPort: 22
      - hostPort: 8080
        vmPort: 80
```

Equivalent CLI flag: `--port 2222:22`

---

### `spec.cloudInit` (optional)

Cloud-init NoCloud configuration injected as a `cidata` block device at boot. Requires the guest OS to have `cloud-init` installed (Fedora Cloud, Ubuntu Cloud, etc.).

| Field      | Type       | Description |
|------------|------------|-------------|
| `user`     | string     | Username to create; defaults to `podvirt` when omitted |
| `password` | string     | Password for the configured user; defaults to `podvirt` when omitted |
| `sshKeys`  | []string   | SSH public keys to inject into the configured user's `authorized_keys` |

**Example:**
```yaml
cloudInit:
  user: fedora
  sshKeys:
    - ssh-ed25519 AAAA... user@host
```

Equivalent CLI flags: `--user fedora --ssh-key ~/.ssh/id_ed25519.pub`

---

### `spec.boot` (optional)

Direct kernel boot parameters. Useful for custom kernels or early boot testing. All fields are optional.

| Field     | Type   | Description |
|-----------|--------|-------------|
| `kernel`  | string | Path to kernel image |
| `initrd`  | string | Path to initrd image |
| `cmdline` | string | Kernel command line arguments |

**Example:**
```yaml
boot:
  kernel: /boot/vmlinuz
  initrd: /boot/initramfs.img
  cmdline: "console=ttyS0 root=/dev/vda1"
```

---

### `spec.console` (optional)

| Field  | Type   | Default | Description |
|--------|--------|---------|-------------|
| `type` | string | `serial` | Console type: `vnc`, `serial`, or `auto` |
| `port` | int    | `5900`  | VNC listen port (only used when `type: vnc`) |

**Example:**
```yaml
console:
  type: vnc
  port: 5900
```

---

## CLI Flags

All `create` flags can be used standalone or combined with `--config` to override specific fields. When both are provided, CLI flags take precedence over file values.

### `podvirt create` flags

| Flag               | Default                              | Description |
|--------------------|--------------------------------------|-------------|
| `--config`, `-c`   | —                                    | Path to VM YAML config file |
| `--name`           | —                                    | VM name |
| `--cpus`           | `1`                                  | Number of vCPUs |
| `--memory`         | `1Gi`                                | Memory size (e.g. `2Gi`, `512Mi`) |
| `--disk`           | —                                    | Local root disk image path |
| `--disk-size`      | —                                    | Desired size for the root/first disk (e.g. `20Gi`) |
| `--image`          | —                                    | Container disk image URL |
| `--port`, `-p`     | —                                    | Port forward: `hostPort:vmPort[/proto]` (repeatable, e.g. `2222:22`) |
| `--ssh-key`        | auto-detect `~/.ssh/id_*.pub`        | SSH public key file to inject (repeatable) |
| `--user`           | `podvirt`                            | Cloud-init username to create/use |
| `--password`       | `podvirt`                            | Cloud-init password for that user |
| `--launcher-image` | `quay.io/kubevirt/virt-launcher:v1.7.0` | virt-launcher image to use |

### `podvirt start` flags

| Flag              | Default | Description |
|-------------------|---------|-------------|
| `--wait`          | `false` | Block until VM reaches running state |
| `--wait-timeout`  | `120`   | Seconds to wait when `--wait` is set |

### `podvirt stop` flags

| Flag      | Default | Description |
|-----------|---------|-------------|
| `--force` | `false` | Force-stop immediately |
| `--graceful` | `false` | Attempt graceful ACPI shutdown before forcing |
| `--timeout` | `5` | Seconds to wait before forcing (used with `--graceful`) |

### `podvirt list` flags

| Flag           | Default  | Description |
|----------------|----------|-------------|
| `--state`      | —        | Filter by state: `running`, `stopped`, `all` |
| `--output`, `-o` | `table` | Output format: `table`, `json`, `yaml` |

### `podvirt status` flags

| Flag      | Default | Description |
|-----------|---------|-------------|
| `--watch` | `false` | Continuously refresh status output |

### `podvirt delete` flags

| Flag        | Default | Description |
|-------------|---------|-------------|
| `--force`   | `false` | Skip confirmation prompt |

### `podvirt ssh` flags

| Flag               | Default | Description |
|--------------------|---------|-------------|
| `--user`, `-u`     | auto-detect from VM label | SSH username |
| `--port`, `-p`     | auto-detect from port mappings | Override SSH host port |
| `--identity`       | —       | Path to SSH private key (passed as `-i` to ssh) |

### `podvirt clean-cache` flags

No flags. Removes all cached podvirt data (`~/.cache/podvirt/`).

### `podvirt console` flags

No flags. Attaches to the VM's serial console (detach with **Ctrl-]**).

### Global flags

No global flags.

---

## Flag and File Precedence

When both `--config` and CLI flags are provided:

1. YAML file values are loaded first
2. Any CLI flag that was explicitly set on the command line **overrides** the corresponding file value
3. CLI flags left at their default values do **not** override file values

**Example — override just the CPU count:**
```bash
podvirt create --config my-vm.yaml --cpus 4
```

This uses all values from `my-vm.yaml` but changes the CPU count to 4.

---

## Validation Rules

- VM name must be a valid DNS label: lowercase letters, digits, and hyphens; max 63 characters
- `memory` must be ≥ `128Mi`
- `cpu.cores` must be ≥ 1
- At least one disk must be specified
- `source.image` paths must exist on the host at creation time
- Exactly one of `source.image` or `source.containerImage` must be set per disk
- Network type must be `masquerade` when set
