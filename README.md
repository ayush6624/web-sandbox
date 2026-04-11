# web-sandbox

Firecracker-based microVM devboxes for frontend development. Spin up an isolated Ubuntu VM with a fully configured React/TypeScript environment in under a second.

Think [Lovable](https://lovable.dev) / [e2b](https://e2b.dev) — but self-hosted, on bare metal.

## How it works

```
┌─────────────────────────────────────────────────────────┐
│  Host (Linux + KVM)                                     │
│                                                         │
│  websandbox up                                          │
│       │                                                 │
│       ▼                                                 │
│  ┌──────────────────────────────────────────────┐       │
│  │  Firecracker microVM              172.16.0.2 │       │
│  │                                              │       │
│  │  Ubuntu 24.04 (Noble)                        │       │
│  │  Node.js 22 LTS + pnpm + TypeScript          │       │
│  │  Vite React-TS project (pre-installed)       │       │
│  │                                              │       │
│  │  systemd → vite-dev.service                  │       │
│  │            └─ vite --host 0.0.0.0 :5173      │       │
│  └────────────────┬─────────────────────────────┘       │
│                   │ tap0                                 │
│                   │ NAT + port forward                   │
│              host:5173 → 172.16.0.2:5173                │
└─────────────────────────────────────────────────────────┘
```

Each VM boots from a pre-built ext4 rootfs image with everything already installed — Node.js, pnpm, TypeScript, a scaffolded Vite React-TS project, and all `node_modules`. The Vite dev server starts automatically via systemd on boot. No cold-start npm install, no waiting.

Firecracker provides hardware-level isolation (KVM) with ~125ms boot times and ~5MB memory overhead. Each devbox gets its own kernel, filesystem, and network stack.

## Requirements

- Linux host with **KVM** support (`/dev/kvm` must exist)
- Root access (Firecracker requires it)
- ~4.5 GB disk per devbox (rootfs + kernel + firecracker binary)

## Quick start

### 1. Build and sync to a remote Linux machine

```bash
# Clone and build
git clone https://github.com/ayush6624/web-sandbox.git
cd web-sandbox

# Build the Linux binary and push everything to your server
make sync REMOTE_HOST=your-server
```

### 2. One-time setup (on the remote machine)

```bash
ssh you@your-server
cd ~/web-sandbox

# Install Firecracker + kernel
sudo bash scripts/setup-firecracker.sh
sudo bash scripts/setup-kernel.sh

# Build the devbox rootfs (takes ~5 min, resumable if interrupted)
sudo apt-get install -y debootstrap
sudo bash scripts/build-devbox-rootfs.sh

# Configure host networking (tap device, NAT, port forwarding)
sudo bash scripts/setup-network.sh
```

### 3. Boot a devbox

```bash
sudo ./websandbox up --config configs/devbox.json
```

### 4. Verify

From another terminal:

```bash
curl http://172.16.0.2:5173
# Returns the Vite React app HTML
```

### 5. Stop

```bash
sudo ./websandbox down --config configs/devbox.json
```

## CLI

```
websandbox up      Boot a devbox VM
websandbox down    Stop the running VM
websandbox doctor  Validate the environment (KVM, firecracker, rootfs, networking)
```

### Flags

| Flag | Description |
|------|-------------|
| `--config` | Path to JSON config file (required) |
| `--firecracker` | Override firecracker binary path |
| `--kernel` | Override kernel image path |
| `--rootfs` | Override rootfs image path |
| `--vcpus` | Override vCPU count |
| `--mem-mib` | Override memory in MiB |
| `--socket` | Override Firecracker API socket path |

### Doctor

`websandbox doctor` checks everything needed before booting:

```
$ sudo ./websandbox doctor --config configs/devbox.json
[✓] /dev/kvm exists
[✓] firecracker binary: /usr/local/bin/firecracker
[✓] rootfs: /opt/fc/devbox-rootfs.ext4
[✓] tap device: tap0
[✓] ip forwarding: enabled
```

## Configuration

Default config at `configs/devbox.json`:

```json
{
  "firecracker_bin": "/usr/local/bin/firecracker",
  "kernel_image":    "/opt/fc/vmlinux",
  "rootfs_path":     "/opt/fc/devbox-rootfs.ext4",
  "kernel_args":     "reboot=k panic=1 pci=off root=/dev/vda rw console=ttyS0",
  "vcpus":           2,
  "mem_mib":         1024,
  "socket_path":     "",
  "state_path":      "/tmp/websandbox-state.json",
  "tap_device":      "tap0",
  "mac_address":     "AA:FC:00:00:00:01",
  "guest_cidr":      "172.16.0.2/24",
  "gateway_ip":      "172.16.0.1",
  "nameservers":     "8.8.8.8"
}
```

| Field | Description |
|-------|-------------|
| `firecracker_bin` | Path to the Firecracker binary |
| `kernel_image` | Path to the Firecracker-compatible Linux kernel |
| `rootfs_path` | Path to the ext4 rootfs image |
| `kernel_args` | Kernel boot parameters |
| `vcpus` | Number of virtual CPUs |
| `mem_mib` | Memory allocation in MiB |
| `socket_path` | Firecracker API socket (auto-generated if empty) |
| `state_path` | Where to persist VM state for `down` command |
| `tap_device` | TAP network interface name |
| `mac_address` | Guest MAC address |
| `guest_cidr` | Guest IP address in CIDR notation |
| `gateway_ip` | Gateway IP (host side of tap) |
| `nameservers` | DNS servers for the guest |

## Networking

The VM connects to the host via a TAP device with static IP configuration:

```
Guest (172.16.0.2) ←──tap0──→ Host (172.16.0.1) ←──NAT──→ Internet
```

- **Guest → Internet**: iptables masquerade through the host's default interface
- **Host → Guest**: Direct access via `172.16.0.2`
- **External → Guest**: Port forwarding rule maps `host:5173` → `172.16.0.2:5173`

The guest's IP is set via the kernel `ip=` boot parameter — no DHCP, no delay. DNS is a static `/etc/resolv.conf` pointing to `8.8.8.8`.

`scripts/setup-network.sh` handles all of this. Run it once per host.

## What's in the rootfs

The devbox rootfs is a 4 GB ext4 image built by `scripts/build-devbox-rootfs.sh`:

| Layer | Details |
|-------|---------|
| **Base OS** | Ubuntu 24.04 (Noble) via debootstrap |
| **Runtime** | Node.js 22 LTS, npm |
| **Tools** | pnpm, TypeScript |
| **Project** | Vite React-TS template at `/home/sandbox/app` |
| **Dependencies** | `node_modules` pre-installed (zero cold-start) |
| **Service** | `vite-dev.service` — starts Vite on boot, listens on `0.0.0.0:5173` |
| **Debug** | Root password `devbox`, serial console enabled on `ttyS0` |

Actual disk usage is ~600-800 MB. The image is 4 GB sparse (only allocates blocks as needed).

The build script is **resumable** — if interrupted, re-run it and completed steps are skipped.

## Project structure

```
web-sandbox/
├── cmd/websandbox/
│   ├── main.go              CLI entry point (cobra): up, down, doctor
│   └── helpers.go           Config loading + flag wiring
├── internal/
│   ├── config/
│   │   └── config.go        JSON config parsing with defaults
│   ├── state/
│   │   └── state.go         VM state persistence (socket path, VMID)
│   └── vm/
│       ├── options.go        RunOptions struct
│       ├── machine_linux.go  Firecracker SDK integration + networking
│       └── machine_stub.go   Non-Linux stub (returns ErrLinuxOnly)
├── configs/
│   └── devbox.json          Default VM configuration
├── scripts/
│   ├── setup-firecracker.sh Install Firecracker binary
│   ├── setup-kernel.sh      Download Firecracker-compatible kernel
│   ├── setup-network.sh     Host networking (tap, NAT, port forwarding)
│   └── build-devbox-rootfs.sh  Build the devbox ext4 image
├── Makefile                 Build, sync, remote deployment targets
└── go.mod
```

## Makefile targets

| Target | Description |
|--------|-------------|
| `make build` | Compile locally (uses stub on macOS) |
| `make build-linux` | Cross-compile Linux amd64 binary |
| `make sync` | Build + rsync binary, configs, scripts to remote |
| `make sync-all` | Rsync entire project to remote |
| `make remote-shell` | SSH into the remote machine |
| `make remote-doctor` | Run `websandbox doctor` on remote |
| `make remote-setup` | Install Firecracker + kernel on remote |
| `make remote-setup-devbox` | Build rootfs + setup networking on remote |
| `make remote-up` | Boot the devbox VM on remote |
| `make remote-down` | Stop the devbox VM on remote |

Override the remote target: `make sync REMOTE_USER=you REMOTE_HOST=your-server`

## Developing locally

The project compiles on macOS/Windows via a build stub — all Firecracker calls return `ErrLinuxOnly`. This lets you work on the CLI, config parsing, and state management without a Linux machine:

```bash
go build ./...          # compiles fine on macOS
go test ./...           # tests run (VM operations are stubbed)
```

To actually run VMs, you need Linux with KVM. Use `make sync` to push to a remote machine.

## How Firecracker compares

| | Firecracker | Docker | Traditional VM |
|---|---|---|---|
| **Isolation** | Hardware (KVM) | Process (namespaces) | Hardware (KVM) |
| **Boot time** | ~125ms | ~500ms | ~10-30s |
| **Memory overhead** | ~5 MB | ~10 MB | ~100+ MB |
| **Kernel** | Dedicated per VM | Shared with host | Dedicated per VM |
| **Root filesystem** | Dedicated per VM | Layered (overlayfs) | Dedicated per VM |
| **Attack surface** | Minimal (reduced device model) | Broad (shared kernel) | Broad (full device model) |

Firecracker was built by AWS for Lambda and Fargate. It strips the virtual device model down to the bare minimum — no USB, no GPU, no PCI — giving you VM-level security at container-like speed.

## Portability

To run this setup on a different machine, you need 3 files:

| File | Path | Size |
|------|------|------|
| Firecracker binary | `/usr/local/bin/firecracker` | ~5 MB |
| Linux kernel | `/opt/fc/vmlinux` | ~25 MB |
| Rootfs image | `/opt/fc/devbox-rootfs.ext4` | ~4 GB (sparse) |

Copy them to any Linux host with KVM, run `setup-network.sh`, and `websandbox up`.

> **Note:** The rootfs is mutable — writes inside the VM persist to the ext4 file. To run multiple independent VMs from the same base, copy the rootfs per VM or use a copy-on-write approach (`cp --reflink=always` on btrfs/XFS).

## License

MIT
