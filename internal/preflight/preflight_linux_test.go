//go:build linux

package preflight

import (
	"debug/elf"
	"os"
	"strings"
	"testing"

	pkg "github.com/AitorConS/jerboa/internal/package"
)

// dynamicSystemBinary finds a dynamically linked 64-bit ELF on the host so the
// interpreter/DT_NEEDED closure checks can run against a real binary.
func dynamicSystemBinary(t *testing.T) (bin, interp string, needed []string) {
	t.Helper()
	for _, cand := range []string{"/bin/sh", "/usr/bin/env", "/bin/ls"} {
		f, err := elf.Open(cand)
		if err != nil {
			continue
		}
		in := programInterp(f)
		libs, _ := f.ImportedLibraries()
		class := f.Class
		_ = f.Close()
		if in != "" && len(libs) > 0 && class == elf.ELFCLASS64 {
			return cand, in, libs
		}
	}
	t.Skip("no dynamically linked system binary found")
	return "", "", nil
}

func TestCheckImageDynamicMissingInterpAndLibs(t *testing.T) {
	bin, interp, needed := dynamicSystemBinary(t)

	findings := CheckImage(bin, nil, "")
	if !HasErrors(findings) {
		t.Fatalf("expected errors for dynamic binary with empty image, got %v", findings)
	}

	var interpReported, libReported bool
	for _, f := range findings {
		if strings.Contains(f.Message, interp) {
			interpReported = true
		}
		if strings.Contains(f.Message, needed[0]) {
			libReported = true
		}
	}
	if !interpReported {
		t.Errorf("missing interpreter %s not reported: %v", interp, findings)
	}
	if !libReported {
		t.Errorf("missing library %s not reported: %v", needed[0], findings)
	}
}

func TestCheckImageDynamicFullyResolved(t *testing.T) {
	bin, interp, needed := dynamicSystemBinary(t)

	// Satisfy the closure with the binary itself as the host backing for the
	// interpreter and every needed library: walkNeeded only requires that each
	// name resolves to a readable ELF in the tree, and the binary's own
	// dependencies are already in the visited set when it is re-walked.
	files := []pkg.File{{GuestPath: interp, HostPath: bin}}
	for _, lib := range needed {
		files = append(files, pkg.File{GuestPath: "/lib/" + lib, HostPath: bin})
	}

	findings := CheckImage(bin, files, "")
	if HasErrors(findings) {
		t.Fatalf("expected no errors for fully resolved dynamic binary, got:\n%s", Format(findings))
	}
}

func TestCheckImageDynamicNonELFLibraryPresence(t *testing.T) {
	bin, interp, needed := dynamicSystemBinary(t)

	// A library present in the tree but not readable as an ELF (e.g. an ld
	// linker script) must count as present, not fail the closure.
	files := []pkg.File{{GuestPath: interp, HostPath: bin}}
	script := t.TempDir() + "/libscript.so"
	if err := os.WriteFile(script, []byte("GROUP ( /lib/libc.so.6 )"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, lib := range needed {
		files = append(files, pkg.File{GuestPath: "/lib/" + lib, HostPath: script})
	}

	findings := CheckImage(bin, files, "")
	if HasErrors(findings) {
		t.Fatalf("linker-script library should satisfy presence, got:\n%s", Format(findings))
	}
}
