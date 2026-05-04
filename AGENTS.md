# AGENTS.md — Unikernel Engine

> Docker-like unikernel engine. Forks Nanos (C+ASM kernel), adds Go orchestration layer.
> Stack: Go 1.22+, C, ASM on KVM/QEMU.

## Build Commands

```bash
make build              # compile all Go binaries (uni + unid)
make kernel             # build Nanos fork (requires cross-compiler)
make test               # unit tests
make test-integration   # integration tests (requires KVM: /dev/kvm)
make lint               # golangci-lint
make e2e                # full end-to-end suite
make coverage           # HTML coverage report
```

Single test:
```bash
go test ./internal/vm/... -run TestVMStart -v
go test -tags integration ./tests/integration/... -run TestBoot -timeout 10m
```

## Architecture

```
uni CLI (cobra) → Unix socket → unid daemon → KVM/QEMU wrapper
                                           → Registry client
                                           → Scheduler/orchestrator
                 Nanos kernel (C+ASM fork) ← image loader
```

**CLI (`cmd/uni/`)** — one file per subcommand, cobra, zero business logic, all work delegated to `unid` via Unix socket. Always has `--output json` flag.

**Daemon (`cmd/unid/`)** — persistent process, Unix socket API (JSON-RPC 2.0), cluster-aware scheduling.

**API (`internal/api/`)** — JSON-RPC 2.0 over Unix domain socket. Methods: `VM.Run`, `VM.Stop`, `VM.Kill`, `VM.Signal`, `VM.Remove`, `VM.List`, `VM.Get`, `VM.Logs`, `VM.Inspect`.

**VM Manager (`internal/vm/`)** — KVM/QEMU wrapper. `VM` struct is concurrent-safe (`sync.RWMutex`). State machine: `created → starting → running → stopping → stopped`. KVM ioctls wrapped in testable interfaces — never call ioctls directly in business logic.

**Image System (`internal/image/`)** — custom JSON manifest + raw disk image, content-addressable by SHA256. `uni build` validates ELF magic bytes, runs `mkfs`, computes SHA256, writes to `~/.uni/images/<sha256>/`.

**Registry (`internal/registry/`)** — HTTP image registry (simple, non-OCI). Endpoints: `GET /v2/images`, `GET /v2/images/{ref}`, `GET /v2/images/{ref}/disk`, `POST /v2/images` (multipart), `DELETE /v2/images/{ref}`.

**Volume System (`internal/volume/`)** — named persistent virtio-blk disks at `~/.uni/volumes/<name>/disk.img`. Sparse files via seek+write. Created with `uni volume create`, mounted with `uni run -v name:/guest/path[:ro]`. Survive VM restarts.

**Compose (`internal/compose/`)** — YAML parser + validator. Topological sort via Kahn's algorithm with cycle detection. Writes `.uni-compose-state.json` alongside compose file: `{"project": "...", "services": {"frontend": "<vm-id>", "backend": "<vm-id>"}}`.

**Kernel Tools (`internal/tools/`)** — auto-downloads `mkfs`, `kernel.img`, `boot.img` from GitHub releases to `~/.uni/tools/`. Handles version checking and updates. Platform-specific mkfs resolution.

**Kernel (`kernel/`)** — Nanos fork, C+ASM only. Never touch C from Go directly. Always boot-test changes in QEMU. Add C tests under `kernel/test/` for any new kernel function.

## Key Technical Decisions

| Area | Choice |
|---|---|
| KVM interface | QEMU process wrapper initially; migrate to `/dev/kvm` ioctls in Phase 3+ |
| IPC | Unix domain socket, JSON-RPC 2.0 |
| Logging | `slog` (stdlib) in Go; kernel serial console captured by daemon |
| Config | TOML (daemon), JSON (manifests), YAML (compose) |
| DI | Manual constructor injection — no framework |
| Image format | JSON manifest + raw disk, SHA256 content-addressable |
| Networking | TAP + Linux bridge; internal DNS in `unid` |

## Code Rules

- All errors wrapped with context: `fmt.Errorf("starting vm %s: %w", id, err)`
- No global mutable state — constructor injection only
- Interfaces over concrete types in function signatures
- Functions under 50 lines; extract helpers aggressively
- Every exported symbol needs a godoc comment
- All state transitions logged with `slog`

## Testing

- Unit tests co-located: `internal/vm/vm_test.go`
- Integration tests in `tests/integration/`, tagged `//go:build integration`
- Use `testify/require` (fail fast), `gomock`/`mockery` for mocks
- Table-driven tests for all parser/validator logic
- Target 80%+ coverage on `internal/` and `pkg/`
- Integration tests require self-hosted KVM runner

## CI (GitHub Actions)

| Workflow | Triggers | What it does |
|---|---|---|
| `pr.yml` | PRs to `main` | lint + unit tests + kernel build + integration tests (self-hosted KVM runner) |
| `main.yml` | Push to `main` | lint + unit tests + E2E (TODO: enable) + multi-arch release builds + GitHub Release |
| `kernel-release.yml` | Changes to `kernel/**` | builds kernel + mkfs, publishes versioned tag + rolling `latest` release |
| `nightly.yml` | Daily 02:00 UTC | kernel tests + benchmarks + govulncheck + trivy + failure notification (TODO: webhook) |
| `docs.yml` | Changes to `docs/` | Jekyll build + GitHub Pages deploy |

Self-hosted runner needed for `integration-tests` (`runs-on: [self-hosted, linux, kvm]`). When `/dev/kvm not found`, fix with `sudo usermod -aG kvm $USER` then restart runner.

## Phase Status

Currently in **Phase 6** (Package System) — core runtime complete, package system with archive extraction.

| Phase | Status | Key deliverables |
|---|---|---|
| 0 — Foundation | ✅ done | Nanos fork, CI green, QEMU boots |
| 1 — VM Manager | ✅ done | State machine, QEMU wrapper, Unix socket API, `uni run` |
| 2 — Image System | ✅ done | Manifest, content-addressable store, registry, `uni build/images/rmi/push/pull` |
| 3 — Full CLI | ✅ done | `uni ps/logs/stop/rm/inspect/exec`, `--output json`, 81% cmd/uni coverage |
| 4 — Compose | ✅ done | YAML parser, topological sort, shared volumes, `uni compose up/down/ps/logs` |
| 5 — Complete Runtime | ✅ done | Port mapping, env vars, volumes, named instances, `--attach`, `--ip` (host+guest fw_cfg), `uni cp` (to+from VM), `uni volume`, TAP/bridge networking |
| 6 — Package System | ✅ done | `uni pkg list/search/get/remove`, `--pkg` flag on `uni build`, package index/store, archive extraction, `internal/package/` |
| 7 — Orchestrator | ⬜ | Self-healing, scaling, health checks, `uni scale/status`, internal DNS |
| 8 — Registry & Distribution | ⬜ | OCI-compatible registry, image signing, JWT auth (basic server/client exists) |
| 9 — Build System | ⬜ | Multi-language `uni build` (Go/Node/Python/Rust), `unikernel.toml`, multi-arch |
| 10 — Observability | ⬜ | Prometheus metrics, web dashboard, multi-node cluster, daemon persistence |

Phases must be fully tested and stable before advancing. A phase is not done if tests are skipped, lint fails, or only the happy path works.

## Known Platform Notes

- `Stop()` (graceful) sends SIGTERM → 30s → SIGKILL. On Windows SIGTERM is unsupported; falls back to SIGKILL immediately.
- `isFilePath()` handles Windows drive-letter paths (`C:\...`) in addition to Unix prefixes.
- TAP networking (`internal/network/tap.go`) is `//go:build linux` only.
- Bridge creation (`internal/network/bridge_linux.go`) is `//go:build linux` only.
- `parseSig()` uses integer literals for SIGUSR1/SIGUSR2 (`syscall.Signal(10/12)`) for cross-platform compatibility.
- `volume.ParseSize` uses `strconv.ParseInt` (not `fmt.Sscanf`) — Sscanf accepts trailing junk like `"1X"` silently.
- `gofmt` rejects trailing-spaces alignment in struct literals. When CI flags gofmt, run `gofmt -w` directly rather than guessing the alignment.

## QEMU Command Construction

Build pipeline in `internal/vm/qemu.go::buildCmd`:
- Network priority: `NetworkName` (TAP) > `PortMaps` non-empty (SLIRP `hostfwd`) > `-net none`.
- SLIRP user-mode (`-netdev user,...,hostfwd=tcp::8080-:80`) does not need TAP/bridge or root, works on any platform — preferred for `-p`.
- Env vars are passed via `-fw_cfg name=opt/uni/env,string=KEY=VAL\n…`. The kernel reads this at boot.
- Network config (static IP) is passed via `-fw_cfg name=opt/uni/network,string=IP/CIDR,GATEWAY`. Format: `10.0.0.2/24,10.0.0.1`.
- Volumes attach as extra `-drive file=...,format=raw,if=virtio,index=N` after the boot disk (index 0).

## Kernel Patches (uni-specific additions to Nanos fork)

- **`kernel/src/drivers/fw_cfg.{c,h}`** — QEMU fw_cfg driver, x86-only (uses I/O ports `0x510`/`0x511`). Reads named files (e.g. `opt/uni/env`) by walking the directory at entry `0x0019`. Confirms `"QEMU"` signature before use; safe no-op on bare metal.
- **`kernel/src/unix/env_inject.c`** — `env_inject_from_fw_cfg(root)` reads `opt/uni/env` and merges entries into `root[environment]` tuple. Called from `stage3.c::startup()` before `exec_elf` builds the user stack envp. Compiles on aarch64 too (`#ifdef __x86_64__` guards the body to a stub).
- **`kernel/src/unix/net_inject.c`** — `net_inject_from_fw_cfg(root)` reads `opt/uni/network` and injects static IP configuration (`ipaddr`, `netmask`, `gateway`) into root tuple. `init_network_iface()` picks this up to configure the first ethernet interface instead of DHCP. x86-only (fw_cfg dependency).
- When changing kernel boot order or the manifest tuple structure, the fw_cfg call site is in `kernel/src/kernel/stage3.c::startup` right after `init_management_root` / `init_kernel_heaps_management`. Must run before `exec_elf` reads the environment tuple.

## Versioning

Both the CLI and the kernel are independently versioned with semver.

| Component | Version file | Release tag format | Pipeline |
|---|---|---|---|
| CLI (uni/unid) | `VERSION` | `v0.1.0` | `main.yml` |
| Kernel artifacts | `kernel/VERSION` | `kernel-v0.1.0` | `kernel-release.yml` |

**Rules:**
- Bump `VERSION` before every commit that changes CLI code.
- Bump `kernel/VERSION` before every commit that changes `kernel/`.
- Patch bump (`0.1.0 → 0.1.1`) for fixes; minor bump (`0.1.0 → 0.2.0`) for features.
- Each pipeline publishes an immutable versioned release **and** updates the shared rolling `latest` release, uploading only its own assets (CLI pipeline never touches kernel assets and vice versa).

**Kernel tools cache** (`~/.uni/tools/`):
- `uni build` auto-downloads kernel artifacts on first use via `internal/tools.ResolveMkfs`.
- `uni build` checks for a newer kernel version before building and prompts `[y/N]`.
- `uni kernel check` / `uni kernel update` / `uni kernel list` / `uni kernel use <v>` manage the cached kernel version.
- After bumping `kernel/VERSION` and pushing, wait for `kernel-release.yml` to complete before the new kernel is available to download.

**CLI self-update:**
- `uni upgrade` replaces the running `uni` binary (and `unid` if found alongside it).
- `uni upgrade check` / `uni upgrade list` for inspection without installing.
- Windows: renames the running binary to `.bak` before placing the new one (cannot overwrite a running `.exe` directly).

## Repository Notes

- Default branch: `main`. No `develop` branch despite some workflow references.
- Remote: `AitorConS/UniCli` (renamed). Pushes work but emit a redirect notice — not a hook failure.

## Critical Function/File Index

| What | Where |
|---|---|
| `uni run` flag wiring | `cmd/uni/run.go` |
| Daemon RPC dispatch | `internal/api/server.go::dispatch` |
| QEMU command builder | `internal/vm/qemu.go::buildCmd` + `buildNetArgs`/`buildEnvArgs`/`buildNetworkCfgArgs`/`buildVolumeArgs` |
| Port spec parser | `internal/vm/portmap.go::ParsePortMap` |
| Compose YAML validators | `internal/compose/parser.go::validatePortSpec` / `validateVolumeSpec` |
| Volume disk allocation | `internal/volume/volume.go::allocateDisk` (sparse via seek+write) |
| Kernel envp construction | `kernel/src/unix/exec.c::build_exec_stack` (reads `process_root[environment]`) |
| Boot-time env injection | `kernel/src/kernel/stage3.c::startup` calls `env_inject_from_fw_cfg(root)` |
| Boot-time network injection | `kernel/src/kernel/stage3.c::startup` calls `net_inject_from_fw_cfg(root)` |
| Kernel tools download/cache | `internal/tools/mkfs.go::ResolveMkfs` + `internal/tools/version.go` |
| Kernel version check (build) | `cmd/uni/build.go::checkKernelUpdateForBuild` |
| Network config fw_cfg | `internal/vm/qemu.go::buildNetworkCfgArgs` — `--ip` → `opt/uni/network` |
| Host-side bridge/TAP | `internal/network/bridge_linux.go` — `CreateBridge`, `AttachTAP`, `DestroyBridge` |
| iptables port forwarding | `internal/network/portfwd_linux.go` — DNAT + MASQUERADE with `-i tapName` |
| Package index/store | `internal/package/package.go` — `Store`, `FetchIndex`, `Search`, `Extract`, `ExtractedFiles`, `RemoveAll` |
| `uni pkg` commands | `cmd/uni/pkg.go` — list, search, get, remove (all versions) |
| Package resolution (build) | `cmd/uni/build.go::resolvePackages` — download, extract, list files for manifest |
| `uni cp` (to VM) | `cmd/uni/cp.go::cpToVM` — dump → copy file → mkfs rebuild |
| Compose shared volumes | `internal/compose/types.go::VolumeConfig` + `cmd/uni/compose.go::newComposeUpCmd` |
| CLI self-update | `cmd/uni/upgrade.go::replaceBinary` |
| CLI version (injected at build) | `cmd/uni/main.go::version` — set via `-X main.version` in `main.yml` |

## Internal Packages

| Package | Description |
|---|---|
| `internal/api/` | JSON-RPC 2.0 server/client over Unix socket. VM lifecycle RPC methods. |
| `internal/compose/` | Compose YAML parser, validator, Kahn's topological sort with cycle detection, shared volumes. |
| `internal/image/` | Image build pipeline (ELF validation, mkfs, SHA256, package files) + content-addressable store. |
| `internal/network/` | TAP device + Linux bridge setup, iptables port forwarding. Linux-only (`//go:build linux`). |
| `internal/package/` | Package index fetch, local store, download, extract, search, remove. |
| `internal/registry/` | HTTP image registry server (simple, non-OCI) + push/pull client. |
| `internal/scheduler/` | **Empty stub.** Placeholder for Phase 7 orchestrator. |
| `internal/tools/` | Kernel tools management: download, version check, platform-specific mkfs resolution. |
| `internal/vm/` | Core package: VM lifecycle state machine, QEMU wrapper, port map parser, VM registry store, network cfg via fw_cfg. |
| `internal/volume/` | Named volume management: sparse disk creation, attach/detach as virtio-blk devices. |

## Stub Packages (placeholders for future phases)

| Path | Phase | Purpose |
|---|---|---|
| `internal/scheduler/` | 7 | Health checks, auto-restart, scaling, DNS |
| `pkg/` | 6+ | Public shared libraries |
| `tests/unit/` | — | Empty; unit tests are co-located with source files |
