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

## Prerequisites

### Required

| Dependency | Version | Notes |
|---|---|---|
| QEMU | 7.0+ | `qemu-system-x86_64` must be in PATH |
| Linux kernel | 5.4+ | KVM acceleration (`/dev/kvm`) |

### Optional

| Dependency | Notes |
|---|---|
| Cross-compiler (`x86_64-elf-gcc`, `nasm`) | Only needed to build the Nanos kernel from source |

{: .note }
Windows is supported for development but **KVM is not available**. VMs run via software emulation (slow). For production use, run on a Linux host with `/dev/kvm`.

### Install QEMU

**Ubuntu / Debian**
```bash
sudo apt-get install qemu-system-x86
```

**Fedora / RHEL**
```bash
sudo dnf install qemu-system-x86
```

**macOS**
```bash
brew install qemu
```

**Windows**
```powershell
winget install SoftwareFreedomConservancy.QEMU
```

### Enable KVM (Linux)

```bash
# Check KVM is available
ls -la /dev/kvm

# Add your user to the kvm group if needed
sudo usermod -aG kvm $USER
# Log out and back in for the group to take effect
```

---

## Installation

### Download pre-built binaries

Download the latest release from [GitHub Releases](https://github.com/AitorConS/UniCli/releases/tag/latest):

| Platform | Binary |
|---|---|
| Linux amd64 | `uni-linux-amd64`, `unid-linux-amd64` |
| Linux arm64 | `uni-linux-arm64`, `unid-linux-arm64` |
| Windows amd64 | `uni-windows-amd64.exe`, `unid-windows-amd64.exe` |

```bash
# Linux — download and install
curl -Lo /usr/local/bin/uni   https://github.com/AitorConS/UniCli/releases/latest/download/uni-linux-amd64
curl -Lo /usr/local/bin/unid  https://github.com/AitorConS/UniCli/releases/latest/download/unid-linux-amd64
chmod +x /usr/local/bin/uni /usr/local/bin/unid
```

### Build from source

```bash
git clone https://github.com/AitorConS/UniCli.git
cd UniCli
make build
# Produces: dist/uni  dist/unid
```

---

## Quick Start

### 1. Start the daemon

The daemon (`unid`) must run in the background before you can use the `uni` CLI.

```bash
# Linux (listens on /var/run/unid.sock)
sudo unid --qemu qemu-system-x86_64

# Without sudo (custom socket path)
unid --socket /tmp/unid.sock --qemu qemu-system-x86_64

# With image registry enabled (port 5000)
unid --socket /tmp/unid.sock --registry-addr :5000
```

Keep this terminal open, or run as a background service (see [Running as a Service](#running-as-a-service)).

### 2. Build your first image

You need a **static Linux ELF binary** — compiled for `GOOS=linux` with no dynamic library dependencies.

**Example: Go hello world**

```go
// hello.go
package main

import "fmt"

func main() {
    fmt.Println("Hello from unikernel!")
}
```

**Linux / macOS:**
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o hello hello.go
```

**Windows (PowerShell):**
```powershell
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"
go build -o hello-linux hello.go
```

**Build the unikernel image:**

```bash
# Linux / macOS
uni build ./hello --name hello
# sha256:abc123...  hello:latest
```

```powershell
# Windows
.\uni-windows-amd64.exe build hello-linux --name hello
# sha256:abc123...  hello:latest
```

On first run, `uni build` automatically downloads `mkfs`, `kernel.img`, and `boot.img` from the [latest release](https://github.com/AitorConS/UniCli/releases/tag/latest) into `~/.uni/tools/` (or `%USERPROFILE%\.uni\tools\` on Windows). On Windows, the build step runs through WSL2.

### 3. Run it

```bash
uni run hello:latest
# a3f8c2d1-...

# Check it's running
uni ps
# ID                                    STATE    IMAGE
# a3f8c2d1-7b4e-4a1f-8c2d-1a2b3c4d5e6f  running  hello:latest

# Read the serial console output
uni logs a3f8c2d1
# Hello from unikernel!

# Stop and clean up
uni stop a3f8c2d1
uni rm a3f8c2d1
```

{: .note }
`uni run` takes a built image name (`hello:latest`) or a path to a `.img` disk image file — **not** a raw ELF binary. Always run `uni build` first.

---

## Running as a Service

### systemd (Linux)

Create `/etc/systemd/system/unid.service`:

```ini
[Unit]
Description=Uni Unikernel Daemon
After=network.target

[Service]
ExecStart=/usr/local/bin/unid --socket /var/run/unid.sock --qemu /usr/bin/qemu-system-x86_64
Restart=on-failure
User=root

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now unid
sudo systemctl status unid
```

---

## Next Steps

- [CLI Reference]({% link cli-reference.md %}) — all commands in detail
- [Compose]({% link compose.md %}) — run multi-service stacks
- [Architecture]({% link architecture.md %}) — how it works internally
