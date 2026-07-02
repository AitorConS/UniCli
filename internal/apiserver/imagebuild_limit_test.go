//go:build linux

package apiserver

import (
	"archive/tar"
	"bytes"
	"testing"
)

// tarWithFile builds a one-file tar archive of the given size.
func tarWithFile(t *testing.T, name string, size int) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(size), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(bytes.Repeat([]byte("x"), size)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBuildContextEnforcesTotalLimit(t *testing.T) {
	orig := maxBuildContextBytes
	maxBuildContextBytes = 1 << 10 // 1 KiB
	t.Cleanup(func() { maxBuildContextBytes = orig })

	// A single file over the cap must be rejected, not truncated onto disk.
	over := tarWithFile(t, "big.bin", 4<<10)
	if _, err := extractBuildContext(bytes.NewReader(over), t.TempDir()); err == nil {
		t.Fatal("expected error for over-limit build context, got nil")
	}
}

func TestExtractBuildContextAccumulatesAcrossFiles(t *testing.T) {
	orig := maxBuildContextBytes
	maxBuildContextBytes = 1 << 10 // 1 KiB
	t.Cleanup(func() { maxBuildContextBytes = orig })

	// Two files each under the cap but together over it must still be rejected:
	// the limit is cumulative, not per-frame.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, name := range []string{"a.bin", "b.bin"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 768, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(bytes.Repeat([]byte("y"), 768)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := extractBuildContext(bytes.NewReader(buf.Bytes()), t.TempDir()); err == nil {
		t.Fatal("expected error for cumulative over-limit context, got nil")
	}
}

func TestExtractBuildContextUnderLimitPasses(t *testing.T) {
	small := tarWithFile(t, "ok.bin", 512)
	files, err := extractBuildContext(bytes.NewReader(small), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}
