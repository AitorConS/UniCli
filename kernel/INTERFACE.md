# Kernel / Motor Interface

Documents the contract between the Nanos kernel fork (`kernel/`) and the Go motor layer (`internal/`).

---

## Image Format

A unikernel image is a raw disk image containing:

```
[ MBR / boot sector (512 bytes) ]
[ Stage 2 bootloader             ]
[ Kernel image (kernel.img)      ]
[ Filesystem image (mkfs output) ]
  └── /program        — the ELF binary
  └── /manifest       — boot manifest (tuple format)
```

The filesystem is built by `kernel/tools/mkfs`. It produces a content-addressable block device image.

### Boot Manifest

The manifest is a key-value tuple (Nanos internal format) embedded in the filesystem:

```
(
  program:    "/program"          # path to ELF binary inside image
  arguments:  ("arg1" "arg2")     # argv
  environment: (KEY: "value")     # env vars
  stack_size: 8388608             # optional, default 8 MiB
  mmap:       mmap-entries        # optional memory mappings
)
```

Go parser lives in `internal/image/manifest.go`. Any kernel ABI change here must be reflected there.

---

## Boot Parameters (x86_64 / pc platform)

Kernel entry via multiboot2 or direct ELF load. QEMU invocation:

```sh
qemu-system-x86_64 \
  -m 512M \
  -drive file=<image>,format=raw,if=virtio \
  -netdev tap,id=n0,ifname=tap0,script=no,downscript=no \
  -device virtio-net-pci,netdev=n0 \
  -nographic \
  -serial stdio \
  -enable-kvm
```

Key flags:
- `-drive if=virtio` — kernel expects virtio-blk
- `-netdev tap` — TAP device managed by `unid`
- `-serial stdio` — kernel console captured by daemon for `uni logs`
- `-enable-kvm` — required for production; omit for CI without KVM

---

## Serial Console Protocol

Kernel writes to serial port (0x3f8 / COM1). `unid` reads from QEMU's stdio pipe.

Lines prefixed `[kernel]` are kernel-level messages (boot, panic, faults).
All other output is the application's stdout/stderr.

---

## VM Exit / Shutdown

Kernel shuts down via ACPI power-off (port 0x604 for QEMU). `unid` detects QEMU process exit.

For graceful shutdown: send SIGTERM to QEMU process → kernel catches ACPI event → clean exit.
For forceful shutdown: send SIGKILL to QEMU process after timeout (default 10s).

---

## Virtio Devices

| Device | Usage |
|---|---|
| `virtio-blk` | Root filesystem image |
| `virtio-net` | Network interface (TAP-backed) |
| `virtio-9p` | Optional shared directory mount |

---

## Go Parser Contract (`internal/image/`)

`internal/image/manifest.go` must implement:

```go
type Manifest struct {
    Program     string
    Arguments   []string
    Environment map[string]string
    StackSize   uint64
}

func ParseManifest(data []byte) (*Manifest, error)
func BuildManifest(m *Manifest) ([]byte, error)
```

`internal/image/builder.go` must implement:

```go
// BuildImage creates a bootable disk image from an ELF binary.
// Calls into kernel/tools/mkfs to produce the filesystem.
func BuildImage(binaryPath string, m *Manifest, outPath string) error
```

---

## Image SHA256 Content Addressing

Images are stored as `<sha256>.img` in the local store. The manifest JSON (not the Nanos tuple) used for content addressing:

```json
{
  "version": 1,
  "sha256": "abc123...",
  "program": "/program",
  "created_at": "2024-01-01T00:00:00Z",
  "size_bytes": 67108864
}
```

This JSON manifest is what `uni images` lists and `uni push/pull` transfers.
