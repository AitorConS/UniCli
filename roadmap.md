# Unikernel Engine — Roadmap

> Stability first. Each phase must pass all tests + lint before the next begins. No exceptions.

---

## Current status: Phase 0 — not started

---

## Phase 0 — Foundation & Kernel Fork `Weeks 1–3`

**Goal:** reproducible kernel build, boots hello-world ELF on QEMU.

### Steps

- [ ] 0.1 — Fork Nanos repo into `kernel/`, strip vendor/cloud-specific bits
- [ ] 0.2 — Set up cross-compiler toolchain (x86_64-elf-gcc, nasm)
- [ ] 0.3 — Write `Makefile` targets: `kernel`, `clean`, `test-kernel`
- [ ] 0.4 — Verify kernel boots on `qemu-system-x86_64` (KVM mode)
- [ ] 0.5 — Boot a static hello-world ELF binary end-to-end via QEMU
- [ ] 0.6 — Document kernel/motor interface: image format, boot params
- [ ] 0.7 — Add first C test under `kernel/test/` (boot integrity)
- [ ] 0.8 — CI: `make kernel` passes in GitHub Actions (`ubuntu-latest`)

**Done when:** any developer can clone + run `make kernel && make test-kernel` and get a passing build. QEMU boots ELF. CI green.

---

## Phase 1 — VM Manager (unid core) `Weeks 4–6`

**Goal:** `uni run ./hello` works end-to-end.

### Steps

- [ ] 1.1 — Go module init (`go mod init`), set up `cmd/uni`, `cmd/unid` entrypoints
- [ ] 1.2 — Define `VMManager` interface in `internal/vm/vm.go`
- [ ] 1.3 — Implement QEMU process wrapper (spawn, kill, monitor)
- [ ] 1.4 — VM state machine: `created → starting → running → stopping → stopped`
  - All transitions logged with `slog`
  - `sync.RWMutex` for concurrent access
- [ ] 1.5 — TAP device + Linux bridge setup (`internal/network/tap.go`)
- [ ] 1.6 — Unix socket API: `unid` listens, `uni` connects (JSON-RPC)
- [ ] 1.7 — `uni run <binary>` command (cobra) → delegates to `unid` via socket
- [ ] 1.8 — Unit tests: VM state machine, socket protocol parsing
- [ ] 1.9 — Integration test: spin up VM, assert it started, tear down
- [ ] 1.10 — `make build` produces `uni` + `unid` binaries

**Done when:** `uni run ./hello` works. Unit + integration tests green. CI passes.

---

## Phase 2 — Image System `Weeks 7–9`

**Goal:** build/push/pull unikernel images round-trip.

### Steps

- [ ] 2.1 — Define image manifest format (JSON, versioned) in `internal/image/manifest.go`
- [ ] 2.2 — Image build pipeline: ELF binary → disk image + manifest
- [ ] 2.3 — Content-addressable local store (SHA256 keyed)
- [ ] 2.4 — `uni build`, `uni images`, `uni rmi` commands
- [ ] 2.5 — Registry server (`internal/registry/`): HTTP, OCI-inspired API
- [ ] 2.6 — `uni push` / `uni pull` client
- [ ] 2.7 — Table-driven tests for manifest parser (valid/invalid/missing-fields)
- [ ] 2.8 — Integration test: build → push → pull → run round-trip

**Done when:** full image round-trip works. Image store tested. Registry server tested. 80%+ coverage on `internal/image/`.

---

## Phase 3 — Full CLI `Weeks 10–11`

**Goal:** complete operational CLI with JSON output.

### Steps

- [ ] 3.1 — `uni ps` — list running instances with metadata
- [ ] 3.2 — `uni logs` — stream stdout from VM serial console
- [ ] 3.3 — `uni stop` — graceful shutdown (ACPI signal → timeout → kill)
- [ ] 3.4 — `uni rm` — remove stopped instance + cleanup
- [ ] 3.5 — `uni inspect` — detailed instance info (JSON)
- [ ] 3.6 — `uni exec` — send signals to running instance
- [ ] 3.7 — `--output json` flag on all commands
- [ ] 3.8 — Errors to stderr, output to stdout (enforced in tests)
- [ ] 3.9 — 80%+ unit coverage on `cmd/uni/`

**Done when:** all commands work. JSON output works. Coverage met. CI green.

---

## Phase 4 — Compose & Multi-service `Weeks 12–14`

**Goal:** `uni compose up` starts 2+ services on a virtual network.

### Steps

- [ ] 4.1 — Define compose YAML format (services, networks, volumes)
- [ ] 4.2 — YAML parser + validator (`internal/compose/`)
- [ ] 4.3 — Dependency graph: topological sort for startup ordering
- [ ] 4.4 — Internal virtual network between compose services
- [ ] 4.5 — Shared volumes (virtio-blk backed)
- [ ] 4.6 — `uni compose up / down / logs / ps`
- [ ] 4.7 — E2E test: 2-service compose, services communicate via network

**Done when:** compose up with 2+ services. Inter-service networking works. E2E green.

---

## Phase 5 — Orchestrator `Weeks 15–18`

**Goal:** self-healing, scalable service management.

### Steps

- [ ] 5.1 — Health check probes: TCP + HTTP, configurable interval/threshold
- [ ] 5.2 — Restart policy: on-failure, always, with exponential backoff
- [ ] 5.3 — Auto-restart on crash with backoff
- [ ] 5.4 — Rolling updates: drain old → start new → verify healthy → repeat
- [ ] 5.5 — `uni scale <service>=N`
- [ ] 5.6 — Internal DNS resolver in `unid` for service discovery
- [ ] 5.7 — E2E test: crash a service, verify auto-restart

**Done when:** health checks, restart, scale, DNS all work. E2E green.

---

## Phase 6 — Registry & Distribution `Weeks 19–20`

**Goal:** self-hosted registry, OCI-compatible, with auth.

### Steps

- [ ] 6.1 — OCI-compatible registry API (push/pull/list/delete)
- [ ] 6.2 — Image signing (cosign or custom) + verification on pull
- [ ] 6.3 — Auth: token-based (JWT or similar)
- [ ] 6.4 — `uni push` / `uni pull` with auth headers
- [ ] 6.5 — Layer deduplication in storage
- [ ] 6.6 — Public package index for common runtimes (Go, Python, Node)

**Done when:** self-hosted registry push/pull with auth and signing works.

---

## Phase 7 — Observability & Polish `Weeks 21–24`

**Goal:** production-ready observability and basic multi-node.

### Steps

- [ ] 7.1 — Prometheus metrics endpoint in `unid` (`/metrics`)
- [ ] 7.2 — Structured log export from VM serial console
- [ ] 7.3 — Web dashboard (Go-served, lightweight) — running instances, health
- [ ] 7.4 — Multi-node: basic cluster membership, workload distribution
- [ ] 7.5 — Documentation site

**Done when:** metrics endpoint live. Dashboard shows instances. Basic multi-node works.

---

## Principles (enforced across all phases)

- Phase not done if: tests skipped, lint fails, happy-path only
- Every PR to `main` requires: lint + unit tests + kernel build + integration tests green
- Interfaces before implementations
- No global mutable state
- Functions under 50 lines
- All errors wrapped: `fmt.Errorf("context: %w", err)`
