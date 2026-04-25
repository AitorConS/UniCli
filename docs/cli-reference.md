---
layout: default
title: CLI Reference
nav_order: 3
---

# CLI Reference
{: .no_toc }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Global Flags

Every `uni` command accepts these flags:

| Flag | Default | Description |
|---|---|---|
| `--socket` | `/var/run/unid.sock` (Linux) / `%TEMP%\unid.sock` (Windows) | Path to the `unid` Unix socket |
| `--store` | `~/.uni/images` | Local image store directory |
| `--output` | `table` | Output format: `table` or `json` |

---

## VM Commands

### `uni run`

Create and immediately start a unikernel VM.

```
uni run <image> [flags]
```

`<image>` can be:
- A **file path**: `./myapp.img` — path to a pre-built bootable disk image
- A **name:tag reference**: `hello:latest` — looked up in the local image store

> **Note:** `uni run` requires a bootable disk image, not a raw ELF binary.
> To package a binary into an image first run `uni build --name <name> <binary>`,
> then `uni run <name>:latest`.

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--memory` | `256M` | VM memory (e.g. `256M`, `1G`, `4G`) |
| `--cpus` | `1` | Number of virtual CPUs |

**Examples:**

```bash
# Run from a pre-built disk image file
uni run ./myapp.img --memory 512M --cpus 2

# Run a built image by name
uni run myapp:latest

# Output the VM ID for scripting
ID=$(uni run hello:latest)
echo "Started VM: $ID"
```

**Output:**
```
a3f8c2d1-7b4e-4a1f-8c2d-1a2b3c4d5e6f
```

---

### `uni ps`

List all registered VMs.

```
uni ps
```

**Examples:**

```bash
uni ps
# ID                                    STATE    IMAGE
# a3f8c2d1-7b4e-4a1f-8c2d-1a2b3c4d5e6f  running  hello:latest
# b4e9d3e2-8c5f-5b2g-9d3e-2b3c4d5e6f7a  stopped  api:v2

# JSON output
uni --output json ps
```

**JSON output:**
```json
[
  {
    "id": "a3f8c2d1-7b4e-4a1f-8c2d-1a2b3c4d5e6f",
    "state": "running",
    "image": "hello:latest"
  }
]
```

---

### `uni logs`

Print captured serial console output (stdout + stderr) for a VM.

```
uni logs <id>
```

**Example:**

```bash
uni logs a3f8c2d1
# Hello from unikernel!
# tick 1
# tick 2
```

{: .note }
Logs are buffered in memory by `unid`. They are lost when the daemon restarts.

---

### `uni inspect`

Display full details for a VM as JSON.

```
uni inspect <id>
```

**Example:**

```bash
uni inspect a3f8c2d1
```

```json
{
  "id": "a3f8c2d1-7b4e-4a1f-8c2d-1a2b3c4d5e6f",
  "state": "running",
  "image": "hello:latest",
  "memory": "256M",
  "cpus": 1,
  "created_at": "2026-04-19T10:00:00Z",
  "started_at": "2026-04-19T10:00:01Z"
}
```

---

### `uni stop`

Gracefully stop a running VM.

```
uni stop <id> [--force]
```

**Shutdown sequence (without `--force`):**
1. Send `SIGTERM` to the QEMU process
2. Wait up to **30 seconds** for the VM to exit cleanly
3. Send `SIGKILL` if still running after the grace period

| Flag | Default | Description |
|---|---|---|
| `--force` | `false` | Skip grace period, send `SIGKILL` immediately |

**Examples:**

```bash
# Graceful shutdown
uni stop a3f8c2d1

# Immediate kill
uni stop --force a3f8c2d1
```

---

### `uni rm`

Remove a stopped VM from the registry.

```
uni rm <id>
```

{: .warning }
The VM must be in `stopped` state. Run `uni stop <id>` first.

**Example:**

```bash
uni stop a3f8c2d1
uni rm a3f8c2d1
```

---

### `uni exec`

Send a signal to a running VM process.

```
uni exec <id> --signal <SIG>
```

| Flag | Default | Description |
|---|---|---|
| `--signal` | `SIGTERM` | Signal name (e.g. `SIGTERM`, `SIGHUP`) or number (e.g. `15`) |

**Examples:**

```bash
# Reload configuration (if the app handles SIGHUP)
uni exec a3f8c2d1 --signal SIGHUP

# Send signal by number
uni exec a3f8c2d1 --signal 1
```

**Supported signal names:** `SIGTERM`, `SIGINT`, `SIGKILL`, `SIGHUP`, `SIGQUIT`, `SIGUSR1`, `SIGUSR2`

---

## Image Commands

### `uni build`

Build a unikernel image from a static ELF binary.

```
uni build <binary> [flags]
```

The binary must be a **static Linux ELF** (`GOOS=linux`, no dynamic library dependencies). Go binaries built with `CGO_ENABLED=0 GOOS=linux` are ideal.

| Flag | Default | Description |
|---|---|---|
| `--name` | Binary filename | Image name |
| `--tag` | `latest` | Image tag |
| `--memory` | `256M` | Default VM memory baked into the image |
| `--cpus` | `1` | Default CPU count baked into the image |
| `--mkfs` | *(auto-downloaded to `~/.uni/tools/mkfs`)* | Path to Nanos mkfs binary — overrides auto-download (env: `UNI_MKFS`) |

**Examples:**

```bash
# Basic build
uni build ./hello

# Custom name and tag
uni build ./myapi --name api --tag v1.2.0

# With resource defaults
uni build ./api --name api --tag latest --memory 512M --cpus 2
```

**Output:**
```
sha256:abc123def456...  api:latest
```

---

### `uni images`

List all images in the local store.

```
uni images
```

**Example:**

```bash
uni images
# DIGEST              NAME   TAG     CREATED               SIZE
# sha256:abc123def4   hello  latest  2026-04-19T10:00:00Z  12.0MB
# sha256:def456ghi7   api    v1.2.0  2026-04-19T11:00:00Z  24.3MB
```

---

### `uni rmi`

Remove an image from the local store.

```
uni rmi <ref>
```

`<ref>` can be `name:tag` or `sha256:<hex>`.

**Example:**

```bash
uni rmi hello:latest
# hello:latest
```

---

### `uni push`

Push a local image to a registry.

```
uni push <ref> <registry-url>
```

**Example:**

```bash
uni push hello:latest http://registry.example.com:5000
# pushed hello:latest to http://registry.example.com:5000
```

---

### `uni pull`

Pull an image from a registry into the local store.

```
uni pull <ref> <registry-url>
```

**Example:**

```bash
uni pull hello:latest http://registry.example.com:5000
# sha256:abc123...  hello:latest
```

---

## Compose Commands

See the full [Compose Reference]({% link compose.md %}) for the file format.

### `uni compose up`

Start all services defined in a compose file, in dependency order.

```
uni compose up <compose-file>
```

**Example:**

```bash
uni compose up stack.yaml
# started backend → a3f8c2d1-...
# started frontend → b4e9d3e2-...
```

---

### `uni compose down`

Stop all services from a compose file, in reverse dependency order.

```
uni compose down <compose-file> [--force]
```

| Flag | Default | Description |
|---|---|---|
| `--force` | `false` | SIGKILL immediately |

---

### `uni compose ps`

List the state of all services in a compose stack.

```
uni compose ps <compose-file>
```

```bash
uni compose ps stack.yaml
# SERVICE   ID                                    STATE
# backend   a3f8c2d1-7b4e-4a1f-8c2d-...          running
# frontend  b4e9d3e2-8c5f-5b2g-9d3e-...          running

uni --output json compose ps stack.yaml
```

---

### `uni compose logs`

Print serial console output for a specific service.

```
uni compose logs <compose-file> <service>
```

```bash
uni compose logs stack.yaml backend
```

---

## VM States

```
created → starting → running → stopping → stopped
```

| State | Description |
|---|---|
| `created` | VM registered, QEMU not started yet |
| `starting` | QEMU process being launched |
| `running` | QEMU process alive and booting/running |
| `stopping` | Stop signal sent, waiting for process exit |
| `stopped` | QEMU process has exited |

{: .note }
A VM in `stopped` state can be removed with `uni rm`. It cannot be restarted — create a new VM with `uni run`.
