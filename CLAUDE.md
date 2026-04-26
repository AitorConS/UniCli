# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Docker-like unikernel engine. Forks Nanos (C+ASM kernel), adds Go orchestration layer. Stack: Go 1.22+, C, ASM on KVM/QEMU.

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

**Daemon (`cmd/unid/`)** — persistent process, Unix socket API (JSON-RPC or Protobuf), cluster-aware scheduling.

**VM Manager (`internal/vm/`)** — KVM/QEMU wrapper. `VM` struct is concurrent-safe (`sync.RWMutex`). State machine: `created → starting → running → stopping → stopped`. KVM ioctls wrapped in testable interfaces — never call ioctls directly in business logic.

**Image System (`internal/image/`)** — custom JSON manifest + raw disk image, content-addressable by SHA256. Changes to kernel ABI (image format, boot params) require updating the Go parser here.

**Volume System (`internal/volume/`)** — named persistent virtio-blk disks at `~/.uni/volumes/<name>/disk.img`. Sparse files via seek+write. Created with `uni volume create`, mounted with `uni run -v name:/guest/path[:ro]`. Survive VM restarts.

**Kernel (`kernel/`)** — Nanos fork, C+ASM only. Never touch C from Go directly. Always boot-test changes in QEMU. Add C tests under `kernel/test/` for any new kernel function.

## Key Technical Decisions

| Area | Choice |
|---|---|
| KVM interface | QEMU process wrapper initially; migrate to `/dev/kvm` ioctls in Phase 3+ |
| IPC | Unix domain socket, JSON-RPC or Protobuf |
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

## CI (GitHub Actions — not Jenkins)

- `pr.yml` — lint + unit tests + kernel build + integration tests on every PR to `main`/`develop`
- `main.yml` — E2E + release CLI binaries on merge to `main`; reads `VERSION` for semver tag
- `kernel-release.yml` — builds and releases kernel artifacts on changes to `kernel/**`; reads `kernel/VERSION`
- `nightly.yml` — full kernel suite + benchmarks + `govulncheck` + `trivy`

## Phase Status

Currently in **Phase 5** (Complete Runtime) — core runtime features done, polish pending. Next up: **Phase 6 — Package System**.

| Phase | Status | Key deliverables |
|---|---|---|
| 0 — Foundation | ✅ done | Nanos fork, CI green, QEMU boots |
| 1 — VM Manager | ✅ done | State machine, QEMU wrapper, Unix socket API, `uni run` |
| 2 — Image System | ✅ done | Manifest, content-addressable store, registry, `uni build/images/rmi/push/pull` |
| 3 — Full CLI | ✅ done | `uni ps/logs/stop/rm/inspect/exec`, `--output json`, 81% cmd/uni coverage |
| 4 — Compose | ✅ done | YAML parser, topological sort, `uni compose up/down/ps/logs`, rolling GitHub Release |
| 5 — Complete Runtime | 🟡 mostly done | Port mapping (`-p`), env vars (`-e` via fw_cfg), volumes (`-v`), named instances, `uni volume`. Pending: `--attach`, `--ip`, `uni cp`, integration tests |
| 6 — Package System | ⬜ next | `uni pkg list/search/get/load`, Node.js/Python/Redis/Nginx packages, package index |
| 7 — Orchestrator | ⬜ | Self-healing, scaling, health checks, `uni scale/status`, internal DNS |
| 8 — Registry & Distribution | ⬜ | OCI-compatible registry, image signing, JWT auth |
| 9 — Build System | ⬜ | Multi-language `uni build` (Go/Node/Python/Rust), `unikernel.toml`, multi-arch |
| 10 — Observability | ⬜ | Prometheus metrics, web dashboard, multi-node cluster, daemon persistence |

Phases must be fully tested and stable before advancing. A phase is not done if tests are skipped, lint fails, or only the happy path works.

## Known Platform Notes

- `Stop()` (graceful) sends SIGTERM → 30s → SIGKILL. On Windows SIGTERM is unsupported; falls back to SIGKILL immediately.
- `isFilePath()` handles Windows drive-letter paths (`C:\...`) in addition to Unix prefixes.
- TAP networking (`internal/network/tap.go`) is `//go:build linux` only.
- `parseSig()` uses integer literals for SIGUSR1/SIGUSR2 (`syscall.Signal(10/12)`) for cross-platform compatibility.
- `volume.ParseSize` uses `strconv.ParseInt` (not `fmt.Sscanf`) — Sscanf accepts trailing junk like `"1X"` silently.
- `gofmt` rejects trailing-spaces alignment in struct literals (e.g. `{input: "x", wantErr: true},   // comment`). When CI flags gofmt, run `gofmt -w` directly rather than guessing the alignment.

## QEMU Command Construction

Build pipeline in `internal/vm/qemu.go::buildCmd`:
- Network priority: `NetworkName` (TAP) > `PortMaps` non-empty (SLIRP `hostfwd`) > `-net none`.
- SLIRP user-mode (`-netdev user,...,hostfwd=tcp::8080-:80`) does not need TAP/bridge or root, works on any platform — preferred for `-p`.
- Env vars are passed via `-fw_cfg name=opt/uni/env,string=KEY=VAL\n…`. The kernel reads this at boot (see Kernel Patches below).
- Volumes attach as extra `-drive file=...,format=raw,if=virtio,index=N` after the boot disk (index 0).

## Kernel Patches (uni-specific additions to Nanos fork)

- **`kernel/src/drivers/fw_cfg.{c,h}`** — QEMU fw_cfg driver, x86-only (uses I/O ports `0x510`/`0x511`). Reads named files (e.g. `opt/uni/env`) by walking the directory at entry `0x0019`. Confirms `"QEMU"` signature before use; safe no-op on bare metal.
- **`kernel/src/unix/env_inject.c`** — `env_inject_from_fw_cfg(root)` reads `opt/uni/env` and merges entries into `root[environment]` tuple. Called from `stage3.c::startup()` before `exec_elf` builds the user stack envp. Compiles on aarch64 too (`#ifdef __x86_64__` guards the body to a stub).
- When changing kernel boot order or the manifest tuple structure, the fw_cfg call site is in `kernel/src/kernel/stage3.c::startup` right after `init_management_root` / `init_kernel_heaps_management`. Must run before `exec_elf` reads the environment tuple.

## Compose State

`uni compose up` writes `.uni-compose-state.json` alongside the compose file. Format:
```json
{"project": "myproject", "services": {"frontend": "<vm-id>", "backend": "<vm-id>"}}
```
`uni compose down/ps/logs` reads this file — run `up` first.

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

- The remote was renamed to `AitorConS/UniCli`. Pushes still work but emit a redirect notice — ignore it; not a hook failure.
- Default branch: `main`. No `develop` branch despite some workflow references.
- Self-hosted runner is needed for `integration-tests` (`runs-on: [self-hosted, linux, kvm]`). When that job fails with `/dev/kvm not found`, the cause is the runner user lacking the `kvm` group (`sudo usermod -aG kvm $USER` then restart the runner service) — not a CI config issue.

## Critical Function/File Index

| What | Where |
|---|---|
| `uni run` flag wiring | `cmd/uni/run.go` |
| Daemon RPC dispatch | `internal/api/server.go::dispatch` |
| QEMU command builder | `internal/vm/qemu.go::buildCmd` + `buildNetArgs`/`buildEnvArgs`/`buildVolumeArgs` |
| Port spec parser | `internal/vm/portmap.go::ParsePortMap` |
| Compose YAML validators | `internal/compose/parser.go::validatePortSpec` / `validateVolumeSpec` |
| Volume disk allocation | `internal/volume/volume.go::allocateDisk` (sparse via seek+write) |
| Kernel envp construction | `kernel/src/unix/exec.c::build_exec_stack` (reads `process_root[environment]`) |
| Boot-time env injection | `kernel/src/kernel/stage3.c::startup` calls `env_inject_from_fw_cfg(root)` |
| Kernel tools download/cache | `internal/tools/mkfs.go::ResolveMkfs` + `internal/tools/version.go` |
| Kernel version check (build) | `cmd/uni/build.go::checkKernelUpdateForBuild` |
| CLI self-update | `cmd/uni/upgrade.go::replaceBinary` |
| CLI version (injected at build) | `cmd/uni/main.go::version` — set via `-X main.version` in `main.yml` |
