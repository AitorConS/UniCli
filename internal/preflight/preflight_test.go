package preflight

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkg "github.com/AitorConS/jerboa/internal/package"
)

// writeMinimalELF writes a minimal ELF64 file with the given machine and no
// program headers — enough for elf.Open to parse class/machine. Returns its path.
func writeMinimalELF(t *testing.T, dir, name string, machine elf.Machine, class elf.Class) string {
	t.Helper()
	// ELF64 header is 64 bytes.
	buf := make([]byte, 64)
	copy(buf, elf.ELFMAG)
	buf[elf.EI_CLASS] = byte(class)
	buf[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	buf[elf.EI_VERSION] = 1
	binary.LittleEndian.PutUint16(buf[16:], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(buf[18:], uint16(machine))
	binary.LittleEndian.PutUint32(buf[20:], 1) // e_version
	binary.LittleEndian.PutUint16(buf[52:], 64) // e_ehsize
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, buf, 0o755); err != nil {
		t.Fatalf("write elf: %v", err)
	}
	return p
}

func TestCheckImageNotELF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "program.txt")
	if err := os.WriteFile(p, []byte("not an elf"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := CheckImage(p, nil, "")
	if !HasErrors(findings) {
		t.Fatalf("expected error findings for non-ELF, got %v", findings)
	}
}

func TestCheckImageStatic64BitPasses(t *testing.T) {
	dir := t.TempDir()
	p := writeMinimalELF(t, dir, "program", elf.EM_X86_64, elf.ELFCLASS64)
	findings := CheckImage(p, nil, "")
	if len(findings) != 0 {
		t.Fatalf("expected no findings for static 64-bit ELF, got %v", findings)
	}
}

func TestCheckImage32BitFails(t *testing.T) {
	dir := t.TempDir()
	// A 32-bit header: elf.Open parses it with a 52-byte header; our 64-byte
	// buffer with EI_CLASS=1 is still parseable (extra bytes ignored).
	p := writeMinimalELF(t, dir, "program32", elf.EM_386, elf.ELFCLASS32)
	findings := CheckImage(p, nil, "")
	if !HasErrors(findings) {
		t.Fatalf("expected error findings for 32-bit ELF, got %v", findings)
	}
}

func TestCheckImageUnsupportedArchFails(t *testing.T) {
	dir := t.TempDir()
	p := writeMinimalELF(t, dir, "program-mips", elf.EM_MIPS, elf.ELFCLASS64)
	findings := CheckImage(p, nil, "")
	if !HasErrors(findings) {
		t.Fatalf("expected error findings for MIPS ELF, got %v", findings)
	}
}

func TestCheckImageMissingEntrypoint(t *testing.T) {
	dir := t.TempDir()
	p := writeMinimalELF(t, dir, "node", elf.EM_X86_64, elf.ELFCLASS64)
	files := []pkg.File{{HostPath: filepath.Join(dir, "other.js"), GuestPath: "other.js"}}
	findings := CheckImage(p, files, "app.js")
	if !HasErrors(findings) {
		t.Fatalf("expected error for missing entrypoint, got %v", findings)
	}
}

func TestCheckImageEntrypointPresent(t *testing.T) {
	dir := t.TempDir()
	p := writeMinimalELF(t, dir, "node", elf.EM_X86_64, elf.ELFCLASS64)
	files := []pkg.File{{HostPath: filepath.Join(dir, "app.js"), GuestPath: "app.js"}}
	findings := CheckImage(p, files, "./app.js")
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestGuestTreeLookup(t *testing.T) {
	tree := newGuestTree([]pkg.File{
		{HostPath: "/host/lib/x86_64-linux-gnu/libc.so.6", GuestPath: "lib/x86_64-linux-gnu/libc.so.6"},
		{HostPath: "/host/dir-only", GuestPath: "data", IsDir: true},
	})
	if _, ok := tree.lookup("libc.so.6"); !ok {
		t.Fatal("basename lookup failed")
	}
	if _, ok := tree.lookup("/lib/x86_64-linux-gnu/libc.so.6"); !ok {
		t.Fatal("absolute path lookup failed")
	}
	if _, ok := tree.lookup("libmissing.so"); ok {
		t.Fatal("expected miss for absent library")
	}
	if _, ok := tree.lookup("data"); ok {
		t.Fatal("directories must not satisfy lookups")
	}
}

func TestFormatIncludesHint(t *testing.T) {
	out := Format([]Finding{{Severity: Error, Message: "boom", Hint: "fix it"}})
	if !strings.Contains(out, "boom") || !strings.Contains(out, "fix it") {
		t.Fatalf("unexpected format output: %q", out)
	}
}
