---
layout: default
title: Troubleshooting
nav_order: 8
---

# Troubleshooting — Common Errors
{: .no_toc }

Every error on this page is real: either Jerboa prints it, or the Nanos guest
prints it on the serial console. Errors are grouped by *when* they happen —
during the build, at boot inside the guest, at run time, or on Windows/WSL.
If a concept below is unfamiliar (one-process model, packages, program path),
read [Build Concepts]({% link build-concepts.md %}) first.

`jerboa build` runs **preflight checks** that catch many of the boot-time
errors before the image exists, and `jerboa logs` / `jerboa run --attach`
print an explanation automatically when they recognize a known guest failure —
so in many cases the fix is already on your screen.

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Build-Time Errors

These come from `jerboa build` itself, before any VM exists.

### `unable to detect project language`

Jerboa looks for marker files: `go.mod` (go), `Cargo.toml` (rust),
`package.json` (node), `pyproject.toml` / `requirements.txt` (python). None
was found — or the path points at something that is neither a source project
nor an ELF binary.

**Fix**: pass `--lang <go|node|python|rust|raw>`, or set `lang` in
`unikernel.toml` (`jerboa init` writes one for you). If you meant to build a
prebuilt binary, point `jerboa build` directly at the ELF file.

### `raw build needs a program`

`lang = "raw"` means "don't compile anything — run a binary from a package",
but Jerboa could not figure out *which* binary. Raw builds resolve the program
from, in order: `[program] path` in `unikernel.toml`, then the `Program`/`Args`
declared by the ops package's own `package.manifest`.

**Fix**: add a `[program]` section with the full in-image path:

```toml
[program]
path = "/usr/local/pgsql/bin/postgres"
```

or use an ops package that declares its program (most do), and make sure the
package is actually listed (`pkgs = [...]` or `--pkg`).

### `program not found in resolved packages`

`[program] path` did not match any file shipped by the declared packages.
Matching tries the exact path, then a path suffix, then the basename.

**Fix**: list the package contents to find the real path:

```sh
jerboa pkg get eyberg/postgresql:11.3.0
# the package tree is extracted under the local package store; check the
# build's verbose output (-V) for the resolved file list
```

Then set `[program] path` to the full path as it exists **inside the image**.

### `not an ELF binary` (preflight)

The program you pointed at is not a Linux ELF — commonly a Windows `.exe`, a
macOS Mach-O binary, or a shell script. Only Linux ELF binaries can boot on
Nanos, and there is no shell in the guest to run scripts.

**Fix**: cross-compile for Linux (`GOOS=linux GOARCH=amd64`, or
`cargo build --target x86_64-unknown-linux-musl`), or use a package that ships
a Linux build. If it is a launcher script, find the real binary it eventually
starts and use that.

### `32-bit ELF` / `unsupported architecture` (preflight)

Nanos images boot 64-bit x86_64 (or arm64) binaries only.

**Fix**: rebuild the program for `x86_64` 64-bit Linux.

### `interpreter ... not found in image` / `missing shared libraries` (preflight)

The program is **dynamically linked**: it needs its dynamic loader (e.g.
`/lib64/ld-linux-x86-64.so.2`) and every shared library in its dependency
closure to be inside the image, at the paths the binary expects. Preflight
walked the binary's `DT_NEEDED` entries and could not resolve some of them
against the files being packed.

**Fix**, one of:

- add the package that ships the missing libraries (`--pkg` / `[build] pkgs`)
- build a **static** binary instead (Go with `CGO_ENABLED=0`, Rust with the
  musl target) — static binaries need nothing else
- if importing from Docker, `jerboa pkg from-docker` bundles the library
  closure automatically — prefer it over hand-copying `.so` files

Preflight errors abort the build on purpose: the same problem at boot time
produces a much more cryptic message inside the guest. If you are certain the
check is wrong, `--no-preflight` skips it.

### `pip install` fails or wheels are missing (python builds)

Python builds install dependencies as **Linux x86_64 wheels** regardless of
your host OS (the guest is Linux even when you build from Windows or macOS).
Packages that only ship source distributions (no manylinux wheel) cannot be
compiled for the guest.

**Fix**: pin a version of the dependency that publishes manylinux wheels, or
vendor a pure-Python alternative. Delete `packages/` (or
`packages/.jerboa-deps-stamp`) to force a clean dependency reinstall.

### `can't find crate` / linker errors mentioning `musl` (rust builds)

The Rust driver targets `x86_64-unknown-linux-musl` to produce a static
binary, and that target is not installed.

**Fix**:

```sh
rustup target add x86_64-unknown-linux-musl
```

### `package not found in index`

The package reference does not exist in the selected source, or the source is
wrong. The default source is `ops` (`repo.ops.city`); ops refs always have a
namespace: `eyberg/postgresql:11.3.0`, not `postgresql:11.3.0`.

**Fix**: search first, then copy the exact ref:

```sh
jerboa pkg search postgres
jerboa pkg get eyberg/postgresql:11.3.0
```

Locally created packages (`pkg create`, `pkg from-docker`) live in the
`jerboa` source — build with `--pkg-source jerboa` (or `pkg_source = "jerboa"`)
to use them.

---

## Boot-Time Errors (Guest Serial Output)

These appear in `jerboa logs <id>` or `jerboa run --attach` output. The CLI
recognizes all of them and prints the explanation inline — this section is the
long form.

### `popen failure: Cannot allocate memory`

Despite the wording, this is **not** an out-of-memory condition. The program
tried to launch a child process (`fork`/`exec`), and Nanos is a
single-process kernel: there is no fork, no exec, no shell. Anything that
shells out — `initdb`, Docker-style entrypoint scripts, process managers,
"worker pool via fork" designs — cannot run in a unikernel.

**Fix**: do the work at build time instead. For PostgreSQL, use the
`eyberg/postgresql` package, which ships a **pre-initialized data directory**
baked into the image, so `initdb` never needs to run. In general: use a
package that ships the pre-computed result, or configure the program to work
in-process (threads are fine; processes are not).

### `unimplemented syscall`

The program used a Linux syscall Nanos does not implement — most often process
management (`fork`, `exec`, `clone` for processes) or an exotic `ioctl`.

**Fix**: check whether the program can be configured not to spawn helpers or
daemonize (e.g. run "in foreground" modes, disable worker processes). If the
syscall is essential to the program's design, it is not unikernel-compatible.

### `could not locate my own executable path`

The program looks up its own path (`/proc/self/exe`) to find its install
prefix (`../share`, `../lib`) — PostgreSQL and MySQL both do this — but it was
executed from a different path than its install tree expects. The usual cause:
`[program] path` was a bare name (`postgres`) that matched a same-named
**launcher stub at the image root** instead of the real binary.

**Fix**: set the **full in-image path** in `unikernel.toml`:

```toml
[program]
path = "/usr/local/pgsql/bin/postgres"   # not just "postgres"
```

### `error loading shared library ...` / `cannot open shared object file`

A shared library the program needs is missing from the image (or present at
the wrong path). This is the boot-time version of the preflight "missing
shared libraries" error — if you skipped preflight with `--no-preflight`,
this is what it was protecting you from.

**Fix**: add the package that ships the library, or switch to a statically
linked binary. Rebuild and let preflight verify the closure resolves.

### `no space left on device`

The root image filesystem is full. By default the image is sized to its
contents — **there is zero free space** for logs, temp files, or database
writes.

**Fix**: reserve room at build time:

```toml
[build]
disk_size = "1G"
```

For data that must also *persist*, use a volume instead of (or in addition to)
free image space.

### `read-only file system` / writes to a missing directory fail

The program wrote to a path that does not exist in the image. Nanos cannot
create top-level directories at run time that were never packed.

**Fix**: declare every directory the program writes to:

```toml
[build]
dirs = ["/data", "/tmp"]
```

`dirs` is also required for **volume mount points** — a volume can only be
mounted onto a directory that already exists in the image.

### `out of memory` / `failed to allocate`

The guest genuinely ran out of RAM (unlike `popen failure`, which only
mentions memory). The default VM memory is 256M.

**Fix**: raise it per run or bake a default into the image:

```sh
jerboa run app:latest --memory 512M
```

```toml
[run]
memory = "512M"
```

### The VM exits immediately with no output

The program started and exited before printing anything — commonly: it
expected a config file or environment variable that is missing, it was a
launcher stub that failed silently, or it daemonized (which Nanos cannot do).

**Fix**: build with `--smoke` to catch this class at build time; run with
`--attach` to see everything from the first byte; check `[env]` against what
the program requires; make sure the program runs in **foreground** mode
(e.g. `nginx -g "daemon off;"` style flags — daemonizing forks).

---

## Run-Time Errors

### `--port requires --network`

Port publishing is implemented by the managed network's userspace forwarder;
there is no SLIRP fallback.

**Fix**:

```sh
jerboa network create app
jerboa run web:latest --network app -p 8080:80
```

### UDP port mapping does not forward

UDP mappings are accepted syntactically but the current forwarder skips them
with a warning. Only TCP forwarding works today.

### `pre-existing shared memory block ... is still in use` (PostgreSQL)

PostgreSQL found a stale `postmaster.pid` from an ungraceful shutdown — the
previous VM was killed (`stop --force`, host reboot) instead of stopped
cleanly, and the lock lives on the persistent volume.

**Fix**: always stop database VMs with plain `jerboa stop` so they checkpoint
and clear their lock. To clear an already-stale lock, re-seed the volume
(`jerboa volume seed` — note this resets the data to the seed state).

### The database is empty after mounting a volume

A fresh volume is empty, and mounting an empty volume over a path that has
baked data (e.g. the pre-initialized database at `/db`) **shadows** that data.

**Fix**: seed the volume before first use — or create and seed in one step:

```sh
jerboa volume create pgdata --size 1G \
  --seed-pkg eyberg/postgresql:11.3.0 --src /db
jerboa run postgresql -v pgdata:/db --network pgnet -p 5432:5432
```

### `mount point does not exist`

Volumes mount onto directories that must already exist inside the image.

**Fix**: add the path to `[build] dirs` and rebuild:

```toml
[build]
dirs = ["/db"]
```

---

## Windows / WSL Errors

### `daemon not installed` / daemon-backed commands fail

On Windows the daemon runs inside a dedicated WSL2 distro named `jerboa`.

**Fix**:

```powershell
jerboa daemon install
jerboa daemon start
jerboa daemon status
```

Daemon-backed commands auto-start the daemon when needed; purely local
commands (`init`, `pkg list`, …) never touch it.

### Connection refused / talks to the wrong daemon on port 7890

The Windows client dials `tcp://127.0.0.1:7890` by default. If **another**
process (for example a leftover `jerboad` installed manually in a different
WSL distro) is listening on that port, the CLI reaches the wrong daemon and
sees none of your images or VMs.

**Fix**: find and stop the stray daemon inside other distros
(`wsl -d <distro> -- pkill jerboad`), or point the client explicitly at the
right endpoint with `-H`/`JERBOA_HOST`. `jerboa daemon status` shows the
endpoint the managed distro actually uses.

### Firecracker fails inside WSL

Firecracker requires KVM (`/dev/kvm`) inside the WSL2 distro, which needs
nested virtualization support on the host.

**Fix**: use QEMU (the default) — it falls back to TCG emulation without KVM —
or enable nested virtualization for WSL2.

---

## Still Stuck?

- Re-run the build with `-V/--verbose` to see the raw driver and packaging
  output, and read the preflight report.
- `jerboa build . --name app --smoke` boots the image once right after
  building and reports known failure signatures automatically.
- `jerboa inspect <id>` dumps the full VM state as JSON.
- [Build Concepts]({% link build-concepts.md %}) explains the model behind
  most of these errors.
