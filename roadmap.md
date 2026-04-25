# Unikernel Engine — Roadmap

> Stability first. Each phase must pass all tests + lint before the next begins. No exceptions.

---

## Current status: Phase 4 — complete

---

## Phase 0 — Foundation & Kernel Fork `Weeks 1–3`

**Goal:** reproducible kernel build, boots hello-world ELF on QEMU.

### Steps

- [x] 0.1 — Fork Nanos repo into `kernel/`, strip vendor/cloud-specific bits (AWS, HyperV, VMware, Xen, riscv64)
- [x] 0.2 — Set up cross-compiler toolchain (x86_64-elf-gcc, nasm) — runs in CI, verify locally on Linux runner
- [x] 0.3 — Write `Makefile` targets: `kernel`, `clean`, `test-kernel`
- [x] 0.4 — Verify kernel boots on `qemu-system-x86_64` (KVM mode) — needs CI green first
- [x] 0.5 — Boot a static hello-world ELF binary end-to-end via QEMU — `tests/e2e/phase0_boot_test.go`
- [x] 0.6 — Document kernel/motor interface: image format, boot params → `kernel/INTERFACE.md`
- [x] 0.7 — Add C test suite under `kernel/test/` (full Nanos unit suite imported)
- [x] 0.8 — CI: `make kernel` passes in GitHub Actions (`ubuntu-latest`) — pending first push + CI run

**Done when:** any developer can clone + run `make kernel && make test-kernel` and get a passing build. QEMU boots ELF. CI green.

---

## Phase 1 — VM Manager (unid core) `Weeks 4–6`

**Goal:** `uni run ./hello` works end-to-end.

### Steps

- [x] 1.1 — Go module init (`go mod init`), set up `cmd/uni`, `cmd/unid` entrypoints
- [x] 1.2 — Define `VMManager` interface in `internal/vm/vm.go`
- [x] 1.3 — Implement QEMU process wrapper (spawn, kill, monitor)
- [x] 1.4 — VM state machine: `created → starting → running → stopping → stopped`
  - All transitions logged with `slog`
  - `sync.RWMutex` for concurrent access
- [x] 1.5 — TAP device + Linux bridge setup (`internal/network/tap.go`)
- [x] 1.6 — Unix socket API: `unid` listens, `uni` connects (JSON-RPC)
- [x] 1.7 — `uni run <binary>` command (cobra) → delegates to `unid` via socket
- [x] 1.8 — Unit tests: VM state machine, socket protocol parsing
- [x] 1.9 — Integration test: spin up VM, assert it started, tear down
- [x] 1.10 — `make build` produces `uni` + `unid` binaries

**Done when:** `uni run ./hello` works. Unit + integration tests green. CI passes.

---

## Phase 2 — Image System `Weeks 7–9`

**Goal:** build/push/pull unikernel images round-trip.

### Steps

- [x] 2.1 — Define image manifest format (JSON, versioned) in `internal/image/manifest.go`
- [x] 2.2 — Image build pipeline: ELF binary → disk image + manifest
- [x] 2.3 — Content-addressable local store (SHA256 keyed)
- [x] 2.4 — `uni build`, `uni images`, `uni rmi` commands
- [x] 2.5 — Registry server (`internal/registry/`): HTTP, OCI-inspired API
- [x] 2.6 — `uni push` / `uni pull` client
- [x] 2.7 — Table-driven tests for manifest parser (valid/invalid/missing-fields)
- [x] 2.8 — Integration test: build → push → pull → run round-trip

**Done when:** full image round-trip works. Image store tested. Registry server tested. 80%+ coverage on `internal/image/`.

---

## Phase 3 — Full CLI `Weeks 10–11`

**Goal:** complete operational CLI with JSON output.

### Steps

- [x] 3.1 — `uni ps` — list running instances with metadata
- [x] 3.2 — `uni logs` — stream stdout from VM serial console
- [x] 3.3 — `uni stop` — graceful shutdown (SIGTERM → 30s timeout → kill)
- [x] 3.4 — `uni rm` — remove stopped instance + cleanup
- [x] 3.5 — `uni inspect` — detailed instance info (JSON)
- [x] 3.6 — `uni exec` — send signals to running instance
- [x] 3.7 — `--output json` flag on all commands
- [x] 3.8 — Errors to stderr, output to stdout (enforced in tests)
- [x] 3.9 — 81% unit coverage on `cmd/uni/`

**Done when:** all commands work. JSON output works. Coverage met. CI green.

---

## Phase 4 — Compose & Multi-service `Weeks 12–14`

**Goal:** `uni compose up` starts 2+ services on a virtual network.

### Steps

- [x] 4.1 — Define compose YAML format (services, networks, volumes)
- [x] 4.2 — YAML parser + validator (`internal/compose/`)
- [x] 4.3 — Dependency graph: topological sort for startup ordering (Kahn's algorithm)
- [x] 4.4 — Internal virtual network between compose services (network refs in YAML)
- [ ] 4.5 — Shared volumes (virtio-blk backed)
- [x] 4.6 — `uni compose up / down / logs / ps`
- [x] 4.7 — E2E test: 2-service compose, services communicate via network

**Done when:** compose up with 2+ services. Inter-service networking works. E2E green.

---

## Phase 5 — Complete Runtime: Ports, Env, Volumes `Weeks 15–17`

**Goal:** `uni run` reaches feature parity with `docker run` for the common 80% of workloads.

The foundation is there (memory, CPUs), but no app that actually listens on a port or reads config
from the environment can be used today. This phase closes that gap before the package system lands,
because packages are useless without a working runtime model.

### 5.1 — Port Mapping

- [x] 5.1.1 — Add `-p / --port host:guest` flag to `uni run` (repeatable, e.g. `-p 8080:80 -p 443:443`)
- [x] 5.1.2 — Implement port forwarding in QEMU wrapper using SLIRP user-mode networking (`-netdev user,hostfwd=...`) as fast path
- [ ] 5.1.3 — TAP/bridge path: add iptables DNAT rules via `internal/network/portfwd.go` (Linux only)
- [x] 5.1.4 — Port map stored in VM config, visible in `uni inspect` and `uni ps --ports`
- [x] 5.1.5 — Expose ports in compose YAML (`ports: ["8080:80"]`) mirroring Docker Compose syntax
- [x] 5.1.6 — Unit tests: port spec parser (ranges, UDP, edge cases); integration test: curl hits a service inside the VM

### 5.2 — Environment Variable Injection

- [x] 5.2.1 — Add `-e / --env KEY=VALUE` flag to `uni run` (repeatable)
- [x] 5.2.2 — Add `--env-file <path>` flag: read `KEY=VALUE` lines from file, identical to Docker
- [x] 5.2.3 — Wire env vars through the API call → QEMU fw_cfg → kernel reads `opt/uni/env`
- [ ] 5.2.4 — Verify env vars are visible inside the VM via `uni exec` / serial log
- [x] 5.2.5 — Env vars in compose YAML (`environment:`) fully functional

### 5.3 — Volume Mounts & Persistent Storage

- [x] 5.3.1 — `internal/volume/` package: create, attach, detach raw virtio-blk disk images
- [x] 5.3.2 — `-v / --volume name:guestpath` flag on `uni run`; named volumes live in `~/.uni/volumes/`
- [x] 5.3.3 — `uni volume create/ls/rm/inspect` subcommands
- [x] 5.3.4 — Volume lifecycle: volumes persist across VM restarts
- [x] 5.3.5 — Read-only mounts: `-v name:guestpath:ro`
- [x] 5.3.6 — Shared volumes between compose services (same volume name in multiple services)
- [ ] 5.3.7 — Integration test: write file in VM, stop, restart, data survives

### 5.4 — Named Instances & UX Polish

- [x] 5.4.1 — `--name <id>` flag on `uni run`; visible in `uni inspect`
- [ ] 5.4.2 — `-d / --detach` flag (default) vs `--attach` (stream serial output to terminal)
- [x] 5.4.3 — `uni run --rm` auto-remove instance on exit
- [ ] 5.4.4 — Static IP assignment: `--ip <addr>` flag (requires TAP networking)
- [ ] 5.4.5 — `uni cp <id>:<guestpath> <localpath>` — copy files out of a running VM (via virtio-serial)

**Done when:** port mapping, env vars, volumes all work. Compose uses them. Integration tests green. 80%+ coverage on new packages.

---

## Phase 6 — Package System `Weeks 18–21`

**Goal:** `uni pkg load node:v20 app.js -p 3000:3000` runs a Node.js app with zero manual compilation.

This is the single biggest usability gap. Right now every app must be a static ELF binary — no
interpreted languages, no dynamic-linked apps, no standard runtimes. The package system provides
pre-compiled, versioned runtime packages (Node.js, Python, Redis, Nginx, …) that can be combined
with user code to produce a ready-to-run unikernel image in one command.

### 6.1 — Package Format & Local Cache

- [ ] 6.1.1 — Define `package.manifest` JSON schema in `internal/pkg/manifest.go`:
  ```json
  {
    "name": "node",
    "version": "v20.11.0",
    "program": "node/node",
    "args": [],
    "runtime": "node",
    "arch": "x86_64",
    "libs": ["lib/libssl.so.3", "lib/libcrypto.so.3"],
    "description": "Node.js JavaScript runtime",
    "homepage": "https://nodejs.org"
  }
  ```
- [ ] 6.1.2 — Local package cache at `~/.uni/packages/<name>/<version>/` (program binary + lib/ dir)
- [ ] 6.1.3 — `internal/pkg/store.go`: install, list, remove, lookup packages by name:version
- [ ] 6.1.4 — Package integrity: SHA256 checksum verified on download, stored in manifest

### 6.2 — `uni pkg` CLI

- [ ] 6.2.1 — `uni pkg list` — list all available packages from index (table: name, version, description)
- [ ] 6.2.2 — `uni pkg search <query>` — regex/substring search across name + description
- [ ] 6.2.3 — `uni pkg get <name:version>` — download package to local cache
- [ ] 6.2.4 — `uni pkg describe <name:version>` — full metadata + included files
- [ ] 6.2.5 — `uni pkg contents <name:version>` — list all files inside the package
- [ ] 6.2.6 — `uni pkg rm <name:version>` — remove from local cache
- [ ] 6.2.7 — `uni pkg ls` — list locally cached packages

### 6.3 — `uni pkg load` — Build & Run with a Runtime

- [ ] 6.3.1 — `uni pkg load <name:version> <app-file> [flags]` — build image from package + user code, then run it
  ```bash
  uni pkg load node:v20 server.js -p 3000:3000
  uni pkg load python:3.12 app.py -e PORT=8080 -p 8080:8080
  uni pkg load nginx:1.24 -v ./html:/var/www/html:ro -p 80:80
  ```
- [ ] 6.3.2 — `--args` flag: extra arguments forwarded to the runtime (e.g. `--args="--max-old-space-size=512"`)
- [ ] 6.3.3 — Bundle pipeline: package binary + libs + user app files → disk image via existing image builder
- [ ] 6.3.4 — `--output-image <name:tag>` flag: save the resulting image instead of running immediately
- [ ] 6.3.5 — `--local` flag: load from a local directory instead of cache

### 6.4 — Package Index Server

- [ ] 6.4.1 — `internal/pkgindex/` HTTP server — serves package manifests + download URLs
- [ ] 6.4.2 — Index JSON format: paginated list of `{name, versions[], latest, description, homepage}`
- [ ] 6.4.3 — Client in `internal/pkg/client.go` hits the index URL (configurable, default `https://packages.unikernel.dev`)
- [ ] 6.4.4 — Offline mode: if index unreachable, fall back to locally cached manifests
- [ ] 6.4.5 — `uni config set packages.index <url>` to point at a self-hosted index

### 6.5 — Official Package Library (first wave)

Build and publish these packages to the official index. Each requires: cross-compiled binary, required
libs bundled, `package.manifest`, integration test.

**Language runtimes:**
- [ ] 6.5.1 — `node:v20` — Node.js 20 LTS (most common web backend runtime)
- [ ] 6.5.2 — `node:v22` — Node.js 22 LTS
- [ ] 6.5.3 — `python:3.12` — CPython 3.12 (static build with stdlib)
- [ ] 6.5.4 — `python:3.11` — CPython 3.11 (LTS for compatibility)
- [ ] 6.5.5 — `ruby:3.2` — MRI Ruby 3.2
- [ ] 6.5.6 — `lua:5.4` — Lua 5.4 (lightweight scripting)
- [ ] 6.5.7 — `php:8.3` — PHP 8.3 CLI

**Web servers:**
- [ ] 6.5.8 — `nginx:1.24` — Nginx static file server + reverse proxy
- [ ] 6.5.9 — `caddy:2.7` — Caddy with automatic HTTPS

**Databases & data stores:**
- [ ] 6.5.10 — `redis:7.2` — Redis in-memory data store
- [ ] 6.5.11 — `sqlite:3.45` — SQLite CLI + library

**Tools:**
- [ ] 6.5.12 — `curl:8.6` — cURL for inter-VM HTTP calls
- [ ] 6.5.13 — `jq:1.7` — JSON processor

### 6.6 — Package Creation Toolchain

- [ ] 6.6.1 — `uni pkg create <name> <binary> [--libs <lib>...]` — scaffold a new package from a local binary
- [ ] 6.6.2 — `uni pkg from-docker <image> --file <binary>` — convert a Docker image into a uni package (extract binary + libs)
- [ ] 6.6.3 — `--missing-files` flag on `uni pkg load`: detect and report missing dynamic libs at build time (uses `ldd` output analysis)
- [ ] 6.6.4 — `uni pkg push <name:version>` — push a locally created package to the index (requires `uni login`)
- [ ] 6.6.5 — CI pipeline for building official packages: cross-compile on GitHub Actions, publish to index on tag

**Done when:** `uni pkg load node:v20 server.js -p 3000:3000` runs a Node.js HTTP server. All first-wave packages published. Integration tests pass for Node, Python, Redis, Nginx.

---

## Phase 7 — Orchestrator `Weeks 22–25`

**Goal:** self-healing, scalable service management.

### Steps

- [ ] 7.1 — Health check probes: TCP + HTTP, configurable interval/threshold
  - Compose syntax: `healthcheck: {test: ["HTTP", "http://localhost:8080/health"], interval: 10s, retries: 3}`
- [ ] 7.2 — Restart policy: `on-failure`, `always`, `unless-stopped` with exponential backoff (max 5 min)
- [ ] 7.3 — Auto-restart on crash: daemon monitors VM exit code, re-applies restart policy
- [ ] 7.4 — Rolling updates: drain old → start new → verify healthy → repeat; zero downtime
- [ ] 7.5 — `uni scale <name>=N` — spawn or kill instances to reach target count
- [ ] 7.6 — Internal DNS resolver in `unid`: service name → IP, round-robin for scaled services
- [ ] 7.7 — `uni status` — cluster-wide view: all services, health, replica count, recent events
- [ ] 7.8 — E2E test: crash a service → verify auto-restart within 30s
- [ ] 7.9 — E2E test: scale web service to 3 → verify 3 instances → scale to 1 → 2 stopped

**Done when:** health checks, restart, scale, DNS, rolling updates all work. E2E green.

---

## Phase 8 — Registry & Distribution `Weeks 26–28`

**Goal:** production-grade, OCI-compatible registry with auth.

### Steps

- [ ] 8.1 — OCI Distribution Spec v1.1 compatible API (push/pull/list/delete, manifests + blobs)
- [ ] 8.2 — Image signing with `cosign` or built-in Ed25519 keypair; signature stored as OCI referrer
- [ ] 8.3 — Signature verification on `uni pull` and `uni run` (configurable: warn / enforce / off)
- [ ] 8.4 — Auth: token-based (JWT, scoped to repo + action); `uni login <registry>` stores credentials
- [ ] 8.5 — TLS: registry server generates self-signed cert on first boot; supports custom cert via config
- [ ] 8.6 — Layer deduplication: blob-level dedup using content-addressable SHA256 (no duplicate blobs)
- [ ] 8.7 — Garbage collection: `unid gc` removes blobs not referenced by any manifest
- [ ] 8.8 — `uni push / pull` work with auth headers and TLS
- [ ] 8.9 — `uni search <registry>/<query>` — search images on remote registry
- [ ] 8.10 — Docker CLI compatibility: `docker push <registry>/<img>` works against a uni registry

**Done when:** OCI-compatible push/pull with auth + signing works. Docker CLI can push to the registry.

---

## Phase 9 — Build System `Weeks 29–31`

**Goal:** `uni build` handles real multi-language projects, not just pre-compiled ELF binaries.

Today `uni build` requires a pre-compiled static ELF. This phase adds language-aware build pipelines
so developers can point at a project directory and get a runnable image.

### Steps

- [ ] 9.1 — `uni build --lang go .` — detect Go project (`go.mod`), build static binary (`CGO_ENABLED=0`), produce image
- [ ] 9.2 — `uni build --lang node .` — detect Node.js project (`package.json`), bundle with `node` package, produce image
- [ ] 9.3 — `uni build --lang python .` — detect Python project (`requirements.txt` / `pyproject.toml`), bundle with `python` package
- [ ] 9.4 — `uni build --lang rust .` — detect Rust project (`Cargo.toml`), `cargo build --release --target x86_64-unknown-linux-musl`
- [ ] 9.5 — Auto-detect language if `--lang` omitted (inspect project files, fail loudly if ambiguous)
- [ ] 9.6 — `Unikernel` config file (`unikernel.toml`) in project root:
  ```toml
  [build]
  lang = "node"
  entrypoint = "src/server.js"
  args = ["--harmony"]

  [run]
  memory = "512M"
  ports = ["3000:3000"]

  [env]
  NODE_ENV = "production"
  ```
- [ ] 9.7 — `uni build` with no flags reads `unikernel.toml` automatically
- [ ] 9.8 — Multi-stage builds: separate build environment from runtime image (reduce image size)
- [ ] 9.9 — `.unignore` file: exclude files from the disk image (like `.dockerignore`)
- [ ] 9.10 — Build cache: skip rebuild if source hash unchanged
- [ ] 9.11 — `uni build --platform linux/amd64,linux/arm64` — multi-arch image output (amd64 + ARM)

**Done when:** Go, Node.js, Python, Rust projects each build and run end-to-end from source with a single `uni build` command.

---

## Phase 10 — Observability & Production Hardening `Weeks 32–36`

**Goal:** production-ready metrics, tracing, multi-node, and a web dashboard.

### Steps

- [ ] 10.1 — Prometheus metrics endpoint in `unid` (`/metrics`): VM count, state transitions, CPU/memory per VM, port forwarding stats
- [ ] 10.2 — OpenTelemetry trace export from `unid`: span per VM lifecycle event, exportable to Jaeger/Tempo
- [ ] 10.3 — Structured log export: daemon aggregates all VM serial console output, exports as JSON lines (ship to Loki/Splunk/stdout)
- [ ] 10.4 — `uni stats <id>` — live CPU%, memory usage, network I/O per VM (polls QEMU QMP monitor)
- [ ] 10.5 — Web dashboard (Go-served, no JS framework): `/ui` on daemon port
  - Running instances with health status
  - Live log tail per VM
  - CPU / memory sparklines
  - Package index browser
- [ ] 10.6 — Resource quotas per VM: cgroup v2 integration for CPU shares + memory hard limit (enforced at kernel level, not just QEMU hint)
- [ ] 10.7 — I/O throttling: `--disk-iops` and `--network-bps` limits via virtio QoS
- [ ] 10.8 — Multi-node basic cluster: `unid --join <peer>` — gossip membership, workload distribution via consistent hashing
- [ ] 10.9 — `uni node ls` — list cluster members with status + resource capacity
- [ ] 10.10 — Daemon state persistence: SQLite-backed VM store; all VMs survive `unid` restart
- [ ] 10.11 — `govulncheck` + `trivy` scan in nightly CI; block release on critical CVEs
- [ ] 10.12 — Documentation site (`docs/`) with guides: getting started, package authoring, compose, API reference

**Done when:** Prometheus scrapes metrics. Dashboard shows live instances. Multi-node distributes workloads. Daemon survives restart.

---

## Principles (enforced across all phases)

- Phase not done if: tests skipped, lint fails, happy-path only
- Every PR to `main` requires: lint + unit tests + kernel build + integration tests green
- Interfaces before implementations
- No global mutable state
- Functions under 50 lines
- All errors wrapped: `fmt.Errorf("context: %w", err)`
- Package first-wave binaries cross-compiled in CI — never hand-compiled locally

---

## Feature matrix

| Feature | Phase | Status |
|---|---|---|
| Port mapping (`-p host:guest`) | 5 | ⬜ next |
| Environment variables (`-e KEY=VAL`) | 5 | ⬜ next |
| Volume mounts (`-v name:path`) | 5 | ⬜ next |
| Named instances (`--name`) | 5 | ⬜ next |
| Package system (`uni pkg list/get/load`) | 6 | ⬜ |
| Node.js runtime package | 6 | ⬜ |
| Python runtime package | 6 | ⬜ |
| Redis / Nginx packages | 6 | ⬜ |
| Health checks + restart policies | 7 | ⬜ |
| Auto-scaling (`uni scale`) | 7 | ⬜ |
| Internal DNS | 7 | ⬜ |
| OCI-compatible registry | 8 | ⬜ |
| Image signing | 8 | ⬜ |
| Registry auth (JWT) | 8 | ⬜ |
| Multi-language `uni build` | 9 | ⬜ |
| `unikernel.toml` project config | 9 | ⬜ |
| Prometheus metrics | 10 | ⬜ |
| Web dashboard | 10 | ⬜ |
| Multi-node cluster | 10 | ⬜ |
| Daemon state persistence | 10 | ⬜ |
