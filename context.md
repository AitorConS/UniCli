# UNIKERNEL ENGINE
### Agent Roadmap & Engineering Spec
`v0.1 — Confidential`

| Stack | Kernel Base | Target |
|---|---|---|
| Go 1.22+ · C · ASM | Nanos (Apache 2.0 fork) | KVM / QEMU · Bare metal |

---

## 1. Context & Vision

This project forks Nanos (Apache 2.0) and builds a complete unikernel engine on top of it — the missing piece in the unikernel ecosystem. The goal is a Docker-like experience for unikernels, with a real orchestrator, stable CLI, compose support, and a self-hosted registry. No vendor lock-in, no cloud dependency, runs on bare KVM.

> **The gap:** `ops` exists but delegates orchestration entirely to cloud providers. There is no "Kubernetes for unikernels". This project fills that gap.

### Why Nanos as the base

- Apache 2.0 — fully forkable, commercially usable
- Mature: boots x86_64, has virtio-net/blk, lwIP, ELF loader, POSIX shim
- Battle-tested in production by NanoVMs customers
- Written in C — clean separation from the Go motor layer
- `ops` (MIT) serves as reference implementation to understand the kernel interface

### What we add on top

- **uni daemon (`unid`)** — persistent daemon managing VM lifecycle, Unix socket API, cluster-aware scheduling
- **uni CLI** — Docker-compatible UX: `build`, `run`, `ps`, `logs`, `stop`, `rm`, `compose`
- **Registry** — self-hosted image store, content-addressable, push/pull
- **Orchestrator** — health checks, auto-restart, rolling updates, service discovery via internal DNS, scale-to-zero

---

## 2. System Architecture

### Component diagram

```
┌─────────────────────────────────────────────────────────┐
│              uni CLI  (Go)                              │
│   build · run · ps · logs · compose · push · pull       │
├─────────────────────────────────────────────────────────┤
│              unid  daemon  (Go)                         │
│  scheduler · health · networking · registry client      │
├──────────────────────┬──────────────────────────────────┤
│  KVM/QEMU wrapper    │  Registry server (Go)            │
│  TAP · bridge · net  │  image store · content addr.     │
├──────────────────────┴──────────────────────────────────┤
│           Nanos kernel  (C + ASM fork)                  │
│  boot · memory · virtio · lwIP · ELF · POSIX shim       │
├─────────────────────────────────────────────────────────┤
│                  KVM / bare metal                       │
└─────────────────────────────────────────────────────────┘
```

### Repository structure

```
unikernel-engine/
├── kernel/          # Nanos fork (C + ASM)
├── cmd/
│   ├── uni/         # CLI entrypoint
│   └── unid/        # Daemon entrypoint
├── internal/
│   ├── vm/          # KVM/QEMU wrapper
│   ├── image/       # Image build + store
│   ├── network/     # TAP, bridge, DNS
│   ├── scheduler/   # Orchestrator core
│   └── registry/    # Registry server
├── pkg/             # Public shared libs
├── tests/
│   ├── unit/
│   ├── integration/
│   └── e2e/
├── .github/
│   └── workflows/
├── Makefile
└── go.mod
```

---

## 3. Feature Roadmap

> ⚠️ Each phase must be fully tested and stable before starting the next. No skipping phases.

### Phase 0 — Foundation & Kernel Fork `Weeks 1–3` `MVP`

- Fork Nanos, verify it boots on KVM/QEMU
- Set up cross-compiler toolchain
- Establish kernel build pipeline (Makefile)
- Boot a hello-world ELF binary end-to-end
- Document kernel/motor interface (image format, boot params)

### Phase 1 — VM Manager (unid core) `Weeks 4–6` `MVP`

- KVM wrapper in Go using `/dev/kvm` ioctl or QEMU process
- VM lifecycle: create, start, stop, destroy
- TAP networking + bridge setup
- Unix socket API between CLI and daemon
- `uni run <binary>` working end-to-end

### Phase 2 — Image System `Weeks 7–9` `Core`

- Image build pipeline (binary → unikernel image)
- Content-addressable local image store
- `uni build`, `uni images`, `uni rmi`
- Image manifest format (JSON, versioned)
- Push/pull to self-hosted registry

### Phase 3 — Full CLI `Weeks 10–11` `Core`

- `uni ps` — list running instances with metadata
- `uni logs` — stream stdout from VM console
- `uni stop` / `uni rm` — graceful shutdown
- `uni inspect` — detailed instance info
- `uni exec` — send signals to running instance

### Phase 4 — Compose & Multi-service `Weeks 12–14` `Core`

- `uni compose` YAML format (inspired by Docker Compose)
- Dependency ordering and startup sequencing
- Internal virtual network between services
- Shared volumes (virtio-blk backed)
- `uni compose up / down / logs`

### Phase 5 — Orchestrator `Weeks 15–18` `Advanced`

- Health checks (TCP/HTTP probe + restart policy)
- Auto-restart on crash with backoff
- Rolling updates with zero downtime
- `uni scale <service>=N`
- Service discovery via internal DNS

### Phase 6 — Registry & Distribution `Weeks 19–20` `Advanced`

- Self-hosted registry server (OCI-compatible API)
- Image signing and verification
- `uni push` / `uni pull` with auth
- Public package index for common runtimes
- Layer deduplication

### Phase 7 — Observability & Polish `Weeks 21–24` `Advanced`

- Metrics export (Prometheus-compatible)
- Structured logging from VM console
- Web dashboard (lightweight, Go-served)
- Multi-node cluster support (basic)
- Documentation site

---

## 4. Testing Strategy

> 🔴 Tests are not optional. Every feature merged must have corresponding tests. The CI gate blocks merges without passing tests.

### Test pyramid

| Layer | Scope |
|---|---|
| **Unit tests** | Pure Go logic: image parsing, manifest validation, CLI arg parsing, scheduler algorithms, network config. Target: 80%+ coverage on `internal/` and `pkg/`. |
| **Integration tests** | Real KVM interactions: spin up a VM, verify it boots, check networking, test the Unix socket API. Run in a Linux VM with KVM enabled. |
| **E2E tests** | Full scenarios: `uni build → uni run → curl endpoint → uni stop`. Validates the whole stack. Run on every PR targeting `main`. |
| **Kernel tests** | C test suite for the Nanos fork: boot integrity, memory alloc, virtio driver correctness. Run separately from Go tests. |

### Go test conventions

- Unit tests live in the same package: `internal/vm/vm_test.go`
- Integration tests in `tests/integration/`, tagged with `//go:build integration`
- Use `testify/require` for assertions — fail fast, clear errors
- Use `gomock` or `mockery` for interface mocks
- Table-driven tests for all parser/validator logic
- Benchmark tests (`go test -bench`) for hot paths: image load, VM start time

### Example test structure

```go
// internal/image/manifest_test.go
func TestManifestParse(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"valid", validJSON, false},
        {"missing version", noVersionJSON, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := ParseManifest(tt.input)
            require.Equal(t, tt.wantErr, err != nil)
        })
    }
}
```

---

## 5. CI/CD Pipeline

### GitHub Actions — not Jenkins

> ✅ Use GitHub Actions. Jenkins requires infrastructure management, a server to maintain, and ops overhead that distracts from the actual project. GitHub Actions is free for open source, zero-config, and integrates natively with the repo.

| | Jenkins | GitHub Actions |
|---|---|---|
| Infrastructure | Dedicated server to maintain | Zero — runs on GitHub |
| Config | Groovy DSL, external | YAML in the repo |
| Cost | Server + ops time | Free for public repos |
| KVM support | Manual plugin setup | Self-hosted runner |
| Verdict | ❌ Overkill here | ✅ Use this |

### Workflow 1 — PR Checks (`.github/workflows/pr.yml`)

Triggers on every pull request to `main` or `develop`.

```yaml
name: PR Checks
on:
  pull_request:
    branches: [main, develop]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - uses: golangci/golangci-lint-action@v4

  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go test ./... -race -coverprofile=coverage.out
      - run: go tool cover -func=coverage.out

  kernel-build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: make kernel

  integration-tests:
    runs-on: [self-hosted, linux, kvm]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Enable KVM
        run: |
          sudo modprobe kvm-intel || sudo modprobe kvm-amd
          sudo chmod 666 /dev/kvm
      - run: go test -tags integration -timeout 10m ./tests/integration/...
```

### Workflow 2 — Main branch (`.github/workflows/main.yml`)

Triggers on push to `main` (after PR merge).

- Full E2E test suite
- Build release binaries (`linux/amd64`, `linux/arm64`)
- Build and push kernel image artifact
- Generate coverage report

### Workflow 3 — Nightly (`.github/workflows/nightly.yml`)

Triggers on cron `02:00 UTC` daily.

- Full kernel test suite (slow, thorough)
- Benchmark suite — detect performance regressions
- Security scan: `govulncheck` + `trivy`
- Notify on Slack/Discord if failures

---

## 6. Agent Instructions

> ⚠️ This section is written directly for the coding agent. Follow these rules strictly on every task.

### General principles

- Always read existing code before writing new code. Understand patterns in use.
- Never break existing tests. If a refactor breaks tests, fix the tests too.
- Every new exported function or type must have a godoc comment.
- All errors must be wrapped with context: `fmt.Errorf("starting vm %s: %w", id, err)`
- No global mutable state. Pass dependencies explicitly (constructor injection).
- Interfaces over concrete types in function signatures.
- Keep functions under 50 lines. Extract helpers aggressively.

### When implementing a feature

1. Start with the interface/contract in `pkg/` or `internal/`
2. Write the unit tests first (TDD) or immediately after the interface
3. Implement the feature to make tests pass
4. Add integration test if the feature touches KVM, filesystem, or network
5. Update the relevant README or doc comment
6. Run `make lint && make test` before marking done

### Kernel (C) work

- Only modify files under `kernel/` — never touch C from Go directly
- Changes to kernel ABI (image format, boot params) require updating the Go parser in `internal/image/`
- Always boot-test changes in QEMU before declaring done
- Add a C test under `kernel/test/` for any new kernel function

### VM Manager (`internal/vm/`)

- The `VM` struct must be safe for concurrent access (use `sync.RWMutex`)
- VM state machine: `created → starting → running → stopping → stopped`
- All state transitions must be logged with structured logger (`slog`)
- KVM ioctls must be wrapped in testable interfaces — never call ioctls directly in business logic

### CLI (`cmd/uni/`)

- Use `cobra` for command structure — one file per subcommand
- CLI must never contain business logic — delegate everything to `unid` via socket
- All commands must have `--output json` flag for scripting
- Error messages go to `stderr`, output to `stdout`

### Makefile targets

| Target | Description |
|---|---|
| `make build` | Compile all Go binaries |
| `make kernel` | Build the Nanos fork |
| `make test` | Run unit tests |
| `make test-integration` | Run integration tests (requires KVM) |
| `make lint` | Run golangci-lint |
| `make e2e` | Run full end-to-end suite |
| `make coverage` | Generate HTML coverage report |

---

## 7. Key Technical Decisions

| Decision | Choice & Rationale |
|---|---|
| Language (motor) | Go 1.22+ — excellent for daemons, concurrency, CLI tooling |
| Language (kernel) | C + ASM — inheriting from Nanos. No Go in the kernel |
| KVM interface | Start with QEMU process wrapper; migrate hot path to direct `/dev/kvm` ioctls in Phase 3+ |
| Networking | TAP devices + Linux bridge. Internal DNS via lightweight resolver in `unid` |
| Image format | Custom manifest (JSON) + raw disk image. Content-addressable by SHA256 |
| IPC | Unix domain socket between `uni` CLI and `unid`. JSON-RPC or Protobuf |
| Logging | Go `slog` (stdlib, structured). Kernel side: serial console captured by daemon |
| Config | TOML for daemon config. JSON for image manifests. YAML for compose files |
| Dependency injection | Manual DI via constructors. No framework. Keeps things testable and explicit |

---

## 8. Definition of Done — per Phase

| Phase | Done when |
|---|---|
| Phase 0 | A binary boots on QEMU via our toolchain. Build is reproducible. Documented. |
| Phase 1 | `uni run ./hello` works. VM starts, runs, stops. Unit + integration tests green. CI passes. |
| Phase 2 | `uni build` + `uni push` + `uni pull` round-trip works. Image store tested. Registry server tested. |
| Phase 3 | `uni ps`, `logs`, `stop`, `rm` all work. CLI has `--output json`. 80%+ unit coverage on CLI layer. |
| Phase 4 | `uni compose up` starts 2+ services that talk to each other over virtual network. |
| Phase 5 | Health checks work. Crashed VM restarts. `uni scale` changes instance count. E2E green. |
| Phase 6 | Registry self-hosted, push/pull from remote, image signing works. |
| Phase 7 | Prometheus metrics endpoint live. Dashboard shows running instances. Multi-node basic clustering. |

> 🔴 A phase is NOT done if: tests are skipped, lint fails, or the feature only works on the happy path. Edge cases and error handling are part of the feature.