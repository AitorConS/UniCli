---
layout: default
title: Architecture
nav_order: 5
---

# Architecture
{: .no_toc }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Overview

Uni is structured as a **client–daemon** system, the same model used by Docker:

```
┌─────────────────────────────────────────────────────────┐
│  uni  (CLI — short-lived process)                       │
│                                                         │
│  build · run · ps · logs · stop · rm · inspect · exec  │
│  compose up · compose down · compose ps · compose logs  │
└──────────────────────────┬──────────────────────────────┘
                           │
                           │  JSON-RPC 2.0 over Unix domain socket
                           │  /var/run/unid.sock
                           │
┌──────────────────────────▼──────────────────────────────┐
│  unid  (daemon — long-running background process)       │
│                                                         │
│  ┌──────────────────┐  ┌──────────────────────────────┐ │
│  │   VM Manager     │  │   Image Registry (HTTP)      │ │
│  │                  │  │                              │ │
│  │  QEMUManager     │  │   GET  /v2/images            │ │
│  │  ┌────────────┐  │  │   POST /v2/images            │ │
│  │  │ VM #1      │  │  │   GET  /v2/images/{ref}      │ │
│  │  │ qemu-sys.. │  │  │   GET  /v2/images/{ref}/disk │ │
│  │  └────────────┘  │  │   DELETE /v2/images/{ref}    │ │
│  │  ┌────────────┐  │  └──────────────────────────────┘ │
│  │  │ VM #2      │  │                                   │
│  │  │ qemu-sys.. │  │  ┌──────────────────────────────┐ │
│  │  └────────────┘  │  │   Image Store                │ │
│  └──────────────────┘  │   ~/.uni/images/             │ │
│                        │   <sha256>/manifest.json     │ │
│                        │   <sha256>/disk.img          │ │
│                        │   refs.json                  │ │
│                        └──────────────────────────────┘ │
└──────────────────────────┬──────────────────────────────┘
                           │  spawns
┌──────────────────────────▼──────────────────────────────┐
│  QEMU processes  (one per running VM)                   │
│                                                         │
│  qemu-system-x86_64                                     │
│    -m 256M                                              │
│    -drive file=disk.img,format=raw,if=virtio            │
│    -nographic -serial stdio -no-reboot                  │
└──────────────────────────┬──────────────────────────────┘
                           │  boots
┌──────────────────────────▼──────────────────────────────┐
│  Nanos Kernel (C + ASM fork)                            │
│  Loads and runs the static ELF application              │
└─────────────────────────────────────────────────────────┘
```

---

## Components

### `uni` CLI (`cmd/uni/`)

The command-line interface. It is a **thin client** — it does no VM management itself. Every command translates directly into a JSON-RPC call to `unid`.

- One `.go` file per subcommand (`run.go`, `ps.go`, `stop.go`, ...)
- Zero business logic — just argument parsing and formatting
- Cobra framework for command routing

### `unid` daemon (`cmd/unid/`)

The long-running background process that owns everything:

- Listens on a Unix domain socket (JSON-RPC 2.0)
- Manages the VM registry (in-memory `Store`)
- Spawns and monitors QEMU processes
- Optionally serves the HTTP image registry

### VM Manager (`internal/vm/`)

Manages the lifecycle of individual VMs:

**State machine:**
```
created → starting → running → stopping → stopped
```

Every transition is atomic (protected by `sync.RWMutex`) and logged with `slog`.

**Key types:**
- `VM` — represents one virtual machine (ID, config, state, timestamps, log buffer)
- `QEMUManager` — implements the `Manager` interface by spawning `qemu-system-x86_64`
- `Store` — thread-safe in-memory registry of all known VMs

**QEMU command built per VM:**
```bash
qemu-system-x86_64 \
  -m 256M \
  -drive file=/path/to/disk.img,format=raw,if=virtio \
  -nographic \
  -serial stdio \
  -no-reboot \
  -net none
```

Serial console output (stdout + stderr from QEMU) is captured into a thread-safe buffer, accessible via `uni logs`.

### Kernel Tools Cache (`internal/tools/`)

The kernel artifacts (`kernel.img`, `boot.img`, `mkfs`) are downloaded from GitHub releases and cached in `~/.uni/tools/`. They are versioned independently from the CLI using semver (`kernel/VERSION` in the repo).

**Download flow:**
1. `uni build` calls `tools.ResolveMkfs()`
2. If tools are absent → `DownloadVersion("latest")` fetches all three artifacts + saves `kernel-version.txt`
3. If tools are present → checks remote version via GitHub API; if newer, prompts `[y/N]` before replacing

**Versioned releases:** each kernel release is tagged `kernel-vX.Y.Z` on GitHub and is immutable. A rolling `latest` release always points to the most recent build. `uni kernel use <v>` downloads from the specific versioned tag.

### Image System (`internal/image/`)

**Content-addressable store** — images are stored by their SHA256 digest:

```
~/.uni/images/
  refs.json                          ← maps "name:tag" → "sha256hex"
  abc123def456.../
    manifest.json                    ← image metadata
    disk.img                         ← raw VM disk
```

**Manifest format** (`manifest.json`):
```json
{
  "schemaVersion": 1,
  "name": "hello",
  "tag": "latest",
  "created": "2026-04-19T10:00:00Z",
  "config": {
    "memory": "256M",
    "cpus": 1
  },
  "diskDigest": "sha256:abc123...",
  "diskSize": 12582912
}
```

**Builder pipeline** (`image.Builder`):
1. Validate ELF magic bytes on the binary
2. Run `mkfs` (Nanos tool) to create a raw disk image containing the binary
3. Compute SHA256 of the disk
4. Write manifest + disk to the store

### API (`internal/api/`)

JSON-RPC 2.0 over a Unix domain socket.

**Methods:**

| Method | Description |
|---|---|
| `VM.Run` | Create + start a VM |
| `VM.Stop` | Graceful or forced stop |
| `VM.Kill` | Immediate SIGKILL |
| `VM.Signal` | Send arbitrary signal |
| `VM.Remove` | Delete a stopped VM |
| `VM.List` | List all VMs |
| `VM.Get` | Get one VM by ID |
| `VM.Logs` | Get captured serial output |
| `VM.Inspect` | Full VM details |

### Compose (`internal/compose/`)

Parses compose YAML files and resolves startup order:

- **Parser** — validates schema (version, service images, dependency refs, network refs)
- **Graph** — Kahn's topological sort algorithm with cycle detection

### Environment Variable Injection

Environment variables passed via `uni run -e KEY=VALUE` reach the guest through QEMU's `fw_cfg` device — no disk rebuild required.

**Flow:**
1. `uni run -e KEY=VAL` → daemon builds `-fw_cfg name=opt/uni/env,string=KEY=VAL\n`
2. QEMU exposes this as a named file on the fw_cfg device (I/O ports `0x510`/`0x511`)
3. At boot, `env_inject_from_fw_cfg()` in the kernel reads `opt/uni/env` and merges entries into the process environment tuple before `exec_elf` builds the user-space stack

This is x86-64 only; the function compiles to a no-op stub on aarch64.

---

## Image Registry

When started with `--registry-addr :5000`, `unid` serves an HTTP registry:

```
GET    /v2/images              list all images
GET    /v2/images/{ref}        get manifest (name:tag or sha256:hex)
GET    /v2/images/{ref}/disk   download raw disk image
POST   /v2/images              push image (multipart: manifest + disk)
DELETE /v2/images/{ref}        remove image
```

This is intentionally simple — not OCI-compliant, designed for internal use between `uni` instances on a local network.

---

## Networking

Each VM can be attached to a TAP interface for network access. The TAP interface is created and bridged on the Linux host.

{: .note }
TAP networking requires Linux and elevated permissions. It is not available on Windows. See `internal/network/tap.go` (Linux-only build tag).

---

## Security Model

- `unid` runs as root (or a privileged user) to spawn QEMU and manage TAP interfaces
- The Unix socket is the trust boundary — only processes that can access the socket file can manage VMs
- Each VM runs in full KVM hardware isolation — a compromised unikernel cannot escape to the host or other VMs
- No shell, no SSH, no dynamic linking inside the unikernel — attack surface is minimal by design
