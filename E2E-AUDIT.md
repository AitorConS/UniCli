# Jerboa E2E Audit — v0.51.1

> **Resolution status (all fixed on branch `e2e-docs-audit`)**
>
> | # | Finding | Resolution |
> |---|---|---|
> | 1 | Windows volume seed split-brain | Already fixed in `main` (F-012); the released v0.51.1 **daemon** predated it. Rebuilt `jerboad` from `main` and redeployed into the distro; `volume seed` on Windows now succeeds (verified: `pgdata seeded 512.5MB`). Ship a daemon/distro release ≥ this commit. |
> | 2 | `stop` on stopped VM errored | Fixed: `Stop`/`Kill` are now idempotent no-ops when already stopped (firecracker + qemu). Regression test `TestQEMUManager_Stop_AlreadyStopped_NoOp`. |
> | 3 | Docs overpromised Windows host port mirroring | Fixed in docs: `getting-started.md` + `cli-reference.md` now state the distro-IP reality and how to opt into `networkingMode=mirrored`. |
> | 4 | `stop`/`kill` idempotency doc | Documented in `cli-reference.md`. |
> | 5 | `rmi` removed in-use image | Fixed: daemon refuses to remove an image any VM (running or stopped) references. Regression test `TestImageRemove_InUse`; documented in `cli-reference.md`. |
> | 6 | Nested/backend-leaking errors | `run <missing>` message de-nested; `node ls` without a cluster now returns a clean actionable app-error (no `(rpc -32601)`). The `firecracker …`/`qemu …` backend prefix on other not-found errors is left as-is (useful for debugging, low value to change). |
> | 7 | Git-Bash `--src /…` mangling | Already handled in `main` (warns); documented. |
>
> Doc-vs-binary drift closed: `pkg load` wording, `run -p` bind-address help, `ps` short help.
>
> Full builds green (linux + windows), `go vet` clean, `cmd/jerboa` + `apiserver` suites pass, new `vm` regression test passes. (`vm` has one pre-existing environment-only failure, `TestLinux_IsCgroupV2Available_True`, unrelated to these changes.)

---


## Full command/flag re-test (every documented command + flag)

Re-ran the entire CLI surface against the fixed stack (CLI + daemon from `main`).
All commands and documented flags were exercised end-to-end.

**Passed as documented:**

- Global: `--version`/`-v`, `--help`, `-H/--host`, `--store`, `--output json`, `-V/--verbose`.
- `version` (+`--channel beta`), `status` (+json), `config get/set` (+invalid value rejected), `kernel check`/`update -y`.
- `init` — every `--lang` (go/node/python/rust/raw), auto-detect, `--force`, refuse-without-force.
- `build` — `--name --tag --memory --cpus --port -f/--file --no-preflight --smoke --lang --platform`; go + node drivers.
- `run` — `--memory --cpus -e/--env --env-file --name --network --ip -p/--port` (incl. `bindaddr:` and `/udp`), `--health-check` (`http:` and `tcp:`), `--restart` (always/on-failure), `--verify` (off/warn/enforce — enforce refuses an unsigned image), `--disk-iops --disk-bps -v/--volume` (rw and `:ro`), `--attach --rm -d/--detach`.
- `ps` (+json), `inspect`, `stats` (+`--watch`/`--interval`), `logs` (+`--follow`), `exec --signal`, `stop` (idempotent on stopped), `rm`.
- `images` (+json), `rmi` (refuses in-use), `sign`, `verify`.
- `network create` (`--subnet --driver`)/`ls`(+json)/`inspect`/`rm`.
- `dns list`/`resolve`/`resolve-all` (+`--network`, +json).
- `volume create` (`--size --seed-pkg --src --pkg-source`)/`seed` (`--pkg --src`)/`ls`/`inspect`/`rm`.
- `pkg list`/`search`/`get`/`remove`/`create` (`--description --runtime --missing-files`)/`push` (error path)/`load` (fixed below).
- `compose up`/`ps`/`logs`/`down --volumes` (auto-creates + tears down the stack network & volume).
- `daemon status`/`logs`/`logs --follow`/`restart`/`start`/`stop`/`install --force`.
- `node ls` — clean "cluster disabled" error.

**Fixed during this pass:**

- **`pkg load` was broken on Windows** — `needsDaemon()` excluded the whole `pkg`
  group, so `pkg load` (which builds+runs via the daemon) dialed the loopback
  default `tcp://127.0.0.1:7890` and failed with "connection refused" instead of
  resolving the WSL distro endpoint. Same class as F-019 (sign/verify). Fixed by
  special-casing `pkg load` in `needsDaemon`; regression asserted in
  `TestNeedsDaemon`. Verified live: `pkg load … --detach` now builds the image.

**Could not exercise here (environment, not a defect):**

- `--cpu-shares` / `--memory-max` — require cgroup v2, unavailable in this WSL2 distro (clear CLI error).
- `pkg from-docker` — Docker Desktop engine was not running.
- `daemon uninstall`/`reinstall` — destructive; `install --force` was exercised during clean setup.

**Minor observations — now fixed:**

- **compose `health_check` http form** — the compose file validator (`SplitN`
  on 2) rejected the `http:PORT:/path` grammar the runtime parser
  (`parseHealthCheck`, `SplitN` on 3) actually requires; the two are now unified
  on the documented `tcp:PORT` / `http:PORT:/path` form. Test updated.
- **`compose down` now removes service VMs** (stop + `Remove`), leaving no
  stopped remnants — matches `docker compose down`.
- **`pkg list` lists both sources by default** — with no `--source` it shows the
  ops and jerboa sections (JSON: `{"ops":[…],"jerboa":[…]}`), so `pkg create`
  packages are visible; `--source ops|jerboa` still filters.
- **`stats` under firecracker** — the firecracker backend now wires the per-VM
  `ProcStatsCollector` (process PID + tap device) like QEMU, so CPU/mem/net read
  from `procfs`/tap instead of reporting the `fallback` source.

**Not a defect (verified):**

- `run --env-file` is validated client-side *before* the VM is created
  (`buildEnv` runs ahead of `api.Dial`), so a bad path fails fast with no orphan
  VM. The earlier "orphan" was an unrelated leftover.
- `pkg list --output-json` uses its own flag (not the global `--output`); left as
  is for backward compatibility.

---

CLI-reference (`docs.jerboa.dev` / `docs/cli-reference.md`) command surface exercised end-to-end against a clean install. Every documented command and its flags were run; problems found are listed below.

## Environment

Clean install from scratch:

| Component | Version | Source |
|---|---|---|
| CLI       | `0.51.1` | built from `main` @ `1159e7b` (`go build -ldflags "-X main.version=0.51.1"`) |
| distro    | `v0.51.1` | `jerboa daemon install --force` (fresh rootfs reimport, downloaded + verified) |
| kernel    | `v0.2.0` | `jerboa kernel update -y` |
| daemon    | `v0.51.1` | inside dedicated `jerboa` WSL2 distro |
| hypervisor| firecracker | default; KVM via `nestedVirtualization=true` |
| WSL       | `2.4.11.0` | networking = default NAT (no `networkingMode=mirrored`) |

## Coverage

Exercised E2E (command + documented flags):

`version`, `status`, `images`, `ps`, `inspect`, `stats`, `logs`, `logs -f`,
`build` (`--name --port --smoke -f --lang(from toml)`), `run`
(`--name --network -p --health-check -e --attach --rm -v --memory --cpus`),
`stop` (`--force` path), `rm`, `exec`, `sign`, `verify`, `rmi`,
`network create/ls/inspect/rm` (`--subnet --driver`),
`volume create/ls/inspect/rm/seed` (`--size --seed-pkg --src --pkg --pkg-source`),
`dns list/resolve/resolve-all`,
`pkg list/search/get/remove` (`--source`),
`config get/set` (`hypervisor`, incl. invalid value),
`kernel check/update`, `node ls`,
`compose ps/up` (error paths),
`daemon install/start/status/uninstall(--force reimport)`.

Full VM lifecycle verified with `examples/hello` (build → smoke → run → logs → inspect → stats → rm → sign → verify → attach+rm) and a networked HTTP server with `examples/webenv` (build `--port` → run `--network -p --health-check -e` → guest DNS → teardown).

---

## Findings

### 1. [HIGH] Windows volumes are split-brain — `volume seed` (and daemon-side volume ops) broken

`jerboa volume create` writes the volume to the **Windows client** store
`C:\Users\aitor\.jerboa\volumes\<name>\` (`disk.img` + `meta.json`), but the
**daemon** store inside the distro (`/root/.jerboa/volumes/`) stays **empty**.
Daemon-side operations therefore cannot see volumes created on the client.

```
$ jerboa volume create seedvol --size 512M
seedvol                                        [exit 0]

$ jerboa volume seed seedvol --pkg eyberg/mysql:5.7.29 --src /var
  ✓ Resolved eyberg/mysql:5.7.29
  ✗ Volume seeding failed
Error: volume seed: resolve volume: volume "seedvol" not found:
       open /root/.jerboa/volumes/seedvol/meta.json: no such file or directory (rpc -32602)   [exit 1]

$ jerboa volume inspect seedvol         # client-side: succeeds
{"id":"seedvol","disk_path":"C:\\Users\\aitor\\.jerboa\\volumes\\seedvol\\disk.img", ...}   [exit 0]
```

Proof the daemon store is empty while the client store has the disk:

```
# Windows client
~/.jerboa/volumes/seedvol/  ->  disk.img (512MB) + meta.json

# distro / daemon
/root/.jerboa/volumes/      ->  EMPTY
```

`volume create / ls / inspect / rm` all operate purely on the client store and
appear to succeed, hiding the disconnect. **Impact:** the documented
database-persistence workflow (`volume create` → `volume seed eyberg/postgresql`
→ `run -v`, in `cli-reference.md` and `getting-started.md`) cannot work on
Windows — seeding fails and a mount would have nothing on the daemon side.

### 2. [MED] `rmi` removes an image still in use by a running VM

While VM `web1` was **running** on image `webenv`:

```
$ jerboa rmi webenv
webenv                                         [exit 0]   # removed anyway
```

Inconsistent with `rm`, which refuses a running VM:

```
$ jerboa rm web1
Error: rm: client remove: firecracker remove web1: vm is running, must be stopped first   [exit 1]
```

Docker refuses `rmi` of an image backing a live container. Here the image is
deleted out from under the running VM, after which `run webenv` and `rmi webenv`
both report "not found". `rmi` should refuse (or require `--force`) when a VM
references the image.

### 3. [MED] Docs overpromise Windows host port access

`docs/getting-started.md:254`:

> By default a published port listens on all interfaces (`0.0.0.0`), so it is
> reachable from the LAN — **and, on Windows, mirrored to the host by WSL2.**

Actual, with default WSL2 NAT networking (no `networkingMode=mirrored` in
`.wslconfig`):

```
$ curl http://127.0.0.1:4333/          # Windows host localhost
[http=000 size=0]                      # connection fails

$ curl http://172.25.150.137:4333/     # distro IP
MESSAGE=e2e-test
[http=200 size=17]                     # works
```

The daemon-side health-check (`--health-check http:4333:/`) reports `healthy`
because it runs *inside* the distro against the guest IP, so the failure is
invisible from `jerboa`'s own view. WSL2 only mirrors ports to the host
`localhost` when `networkingMode=mirrored` is set — it is **not** the default,
and it is not the case for the dedicated `jerboa` distro under NAT.

**Fix:** docs should either require `networkingMode=mirrored` for host
`localhost` access, or tell Windows users to reach published ports via the
distro IP (`jerboa daemon status` → `endpoint` host).

### 4. [LOW] `stop` on an already-stopped VM leaks an internal state error

`hello` self-exits, so the VM is already `stopped`:

```
$ jerboa stop h1
Error: stop: client stop: firecracker stop h1: invalid transition stopped → stopping   [exit 1]
```

Should be a friendly no-op (Docker prints the id and exits 0).

### 5. [LOW] User-facing errors leak the internal hypervisor backend and nest redundantly

```
$ jerboa inspect nope123
Error: inspect: client inspect: firecracker get nope123: vm "nope123" not found

$ jerboa run doesnotexist:latest
Error: run: client run: image "doesnotexist:latest" not found:
       image store get doesnotexist:latest: doesnotexist:latest not found
```

`firecracker …` (backend name) and the triple-nested "not found" are internal
detail. Cosmetic, but noisy for users.

### 6. [LOW] `node ls` without a cluster returns a raw JSON-RPC error

```
$ jerboa node ls
Error: node ls: client node list: method not found: Node.List (cluster disabled) (rpc -32601)   [exit 1]
```

Documented as requiring `--cluster-addr`, but the message exposes JSON-RPC
internals (`rpc -32601`). A clean "cluster is disabled; start the daemon with
`--cluster-addr`" would be friendlier.

### 7. [INFO] Git Bash (MSYS) mangles `--src /...` / path args on Windows

`volume seed --src /` and `build /path` get rewritten by MSYS path conversion
(`/` → `C:/Program Files/Git/`). Jerboa already **detects and warns** for
`--src` (good defensive UX). Not a jerboa bug — worth a docs note that Windows
Git-Bash users need `MSYS_NO_PATHCONV=1` (PowerShell is unaffected).

---

## Doc-vs-binary parity (minor drift)

- `pkg load --help` says "Download a package, build … and **run** the image",
  while `cli-reference.md` says "Download, build, and **prepare** a runnable
  image in one step." Wording drift (run vs prepare).
- `run -p` help shows `host:guest[/tcp|udp]` but omits the documented
  `[bindaddr:]host:guest` bind-address prefix (which does work, e.g.
  `127.0.0.1:host:guest`).
- `ps --help` still says "List running VMs" though it also lists stopped VMs —
  `cli-reference.md` already flags this.

All other documented commands and flags matched the binary and behaved as
described.

---

## Clean-state confirmation (post-teardown)

`ps`, `images`, `network ls`, `volume ls` all empty after teardown — no leaked
VMs, images, networks, or volumes.
