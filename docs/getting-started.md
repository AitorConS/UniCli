---
layout: default
title: Getting Started
nav_order: 2
---

# Getting Started
{: .no_toc }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Platform Model

Jerboa currently runs in two supported ways:

- **Linux host**: `jerboa` talks to a native `jerboad` process over a Unix socket.
- **Windows host**: `jerboa.exe` talks to `jerboad` running inside a dedicated WSL2 distro managed by `jerboa daemon`.

Notes:

- `jerboad` is Linux-only.
- Native VM execution depends on Linux hypervisor support.
- Windows support is built around WSL2, not native Windows virtualization.

## Prerequisites

### Linux

Required for native execution:

- Linux
- QEMU (`qemu-system-x86_64`)
- Go `1.25+` only if you build the CLI from source

Optional:

- `/dev/kvm` for accelerated QEMU; without it, QEMU runs with TCG emulation
- Firecracker if you want `--hypervisor firecracker`
- kernel build toolchain (`gcc-multilib`, `nasm`, `qemu-utils`) only if you build the kernel/toolchain locally

### Windows

Required:

- WSL2
- a Linux distro available to host the dedicated Jerboa distro import
- Go `1.25+` only if you build the CLI from source

The actual daemon and hypervisors run inside the imported `jerboa` WSL2 distro.
Firecracker still requires KVM.

---

## Install

### Linux

For a native Linux host, the repo ships a one-shot installer:

```bash
sudo scripts/install.sh
jerboa status
```

That path provisions the Linux-side runtime, including the daemon service.

If you are building from source instead:

```bash
git clone https://github.com/AitorConS/jerboa.git
cd jerboa
make build
```

Artifacts are written to `dist/`. `make build` builds both binaries; use
`make build-cli` or `make build-daemon` for one side, or `make -j2 build` to
build both in parallel. Source builds use `-trimpath`; `distro/build.sh` also
builds `jerboad` with `-s -w`, making the WSL rootfs artifact about 9 MiB
smaller.

### Windows

On Windows, install the host CLI and then import the dedicated WSL2 distro:

```powershell
jerboa daemon install
jerboa daemon start
jerboa daemon status
```

`jerboa daemon install --rootfs <tarball>` can import a locally built distro rootfs instead of downloading the release artifact.

---

## First Start

### Linux daemon

Typical native daemon start:

```bash
sudo jerboad --host unix:///var/run/jerboad.sock
```

Useful daemon flags:

```bash
sudo jerboad \
  --host unix:///var/run/jerboad.sock \
  --metrics-addr :9090 \
  --ui-addr :8080 \
  --vm-store sqlite
```

### Windows daemon

The daemon lives inside WSL2 and is usually started through the CLI:

```powershell
jerboa daemon start
jerboa daemon logs -f
```

The Windows client auto-starts the daemon for daemon-backed commands when needed.

---

## Build An Image

`jerboa build` requires a reachable daemon. If unikernels are new to you, read
[Build Concepts]({% link build-concepts.md %}) first — it explains the
one-process model, packages, and everything `unikernel.toml` can do.

### Scaffold a project

```bash
jerboa init
```

`jerboa init` detects the project language and writes a fully commented
`unikernel.toml` documenting every field and its pitfalls. Use `--lang raw`
for package-driven builds (databases, prebuilt binaries).

### From a Go project

```bash
jerboa build examples/hello --name hello --lang go
```

Go source builds use size-oriented defaults (`-trimpath` and stripped linker
flags) unless later custom build args override them.

### From a source directory with auto-detection

```bash
jerboa build examples/flaskapp --name flaskapp --port 8080
jerboa build examples/nextapp --name nextapp --port 3000
```

Supported build modes:

- `go`
- `node`
- `python`
- `rust`
- `raw`

Runtime packages (the Node/Python runtimes, database servers, shared
libraries) come from the **ops ecosystem by default** (`--pkg-source ops`);
`unikernel.toml` can override build/run defaults, declare packages
(`pkgs = [...]`), and run pre-build steps.

Before assembling the image, the build runs **preflight checks** (is the
program a bootable Linux ELF? are all its shared libraries in the image?) and
aborts with an explanation and a fix hint when something would fail at boot.
Add `--smoke` to also boot the image once right after building and scan its
output for known failure signatures:

```bash
jerboa build examples/flaskapp --name flaskapp --port 8080 --smoke
```

### From a prebuilt static ELF

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o hello ./examples/hello
jerboa build ./hello --name hello
```

If a build or a boot fails, check [Troubleshooting]({% link troubleshooting.md %})
— it decodes every common error message.

---

## Run A VM

```bash
jerboa run hello:latest
jerboa ps
jerboa logs <vm-id>
```

Attach to serial output:

```bash
jerboa run hello:latest --attach
```

Follow buffered logs:

```bash
jerboa logs <vm-id> -f
```

---

## Networking And Ports

Port publishing is tied to managed networking. There is no SLIRP fallback.

```bash
jerboa network create app
jerboa run myapp:latest --network app -p 8080:80 --name web
jerboa dns list --network app
```

Static IP:

```bash
jerboa run myapp:latest --network app --ip 10.100.0.10 -p 8080:80
```

Important:

- `-p/--port` requires `--network`
- TCP forwarding works today
- UDP port mappings are accepted syntactically but are currently skipped by the forwarder with a warning

### Publish bind address

By default a published port listens on **all interfaces** (`0.0.0.0`), so it is
reachable from the LAN — and, on Windows, mirrored to the host by WSL2. To
restrict a port to the local host, prefix the mapping with a bind address,
Docker-style:

```bash
jerboa run myapp:latest --network app -p 127.0.0.1:8080:80   # localhost only
jerboa run myapp:latest --network app -p 8080:80             # all interfaces
```

---

## Service Discovery (Guest DNS)

VMs on the same network resolve each other by name. The daemon runs a small DNS
server that answers from live VM state, so an app can connect to a peer by its
VM/service name instead of a hardcoded IP:

```bash
jerboa network create app
jerboa run mysql:latest   --network app --name db -p 3306:3306
jerboa run myapp:latest   --network app --name web -p 8080:8080 -e DB_HOST=db
# inside `web`, the hostname `db` resolves to the db VM's IP
```

How it works:

- each image bakes an `/etc/resolv.conf` pointing at a fixed resolver address the
  daemon owns; guests reach it through their default gateway
- queries are scoped by source IP, so `db` resolves to the `db` VM **on the
  caller's own network**
- names the daemon does not own are forwarded to an upstream resolver, so
  ordinary internet lookups still work
- the same mechanism powers `compose` — services connect to each other by
  service name (see [Compose]({% link compose.md %}))

Inspect the records the resolver would return:

```bash
jerboa dns list --network app
jerboa dns resolve db --network app
```

---

## Volumes

```bash
jerboa volume create data --size 1G
jerboa run myapp:latest -v data:/var/data
jerboa volume inspect data
```

A fresh volume is empty — mounting it over a path that has baked data (e.g. a
pre-initialized database) shadows that data. Create and seed in one step:

```bash
jerboa volume create pgdata --size 1G --seed-pkg eyberg/postgresql:11.3.0 --src /db
jerboa run postgresql -v pgdata:/db --network pgnet -p 5432:5432
```

See [Build Concepts → Volumes And Seeding]({% link build-concepts.md %}) for
why seeding is needed.

---

## Compose

Compose stack:

```bash
jerboa compose up stack.yaml
jerboa compose ps stack.yaml
jerboa compose logs stack.yaml api
jerboa compose down stack.yaml --volumes
```

---

## Updating

There is no self-update command for the CLI or daemon binaries.

- update `jerboa` manually from releases
- update `jerboad` manually on Linux, or replace the daemon binary inside the WSL2 distro on Windows
- kernel tooling is managed separately through `jerboa kernel`

Kernel toolchain commands:

```bash
jerboa kernel check
jerboa kernel update
jerboa kernel list
jerboa kernel use v0.1.2
```

---

## Next

- [Build Concepts]({% link build-concepts.md %})
- [CLI Reference]({% link cli-reference.md %})
- [Compose]({% link compose.md %})
- [Architecture]({% link architecture.md %})
- [Observability]({% link observability.md %})
- [Troubleshooting]({% link troubleshooting.md %})
