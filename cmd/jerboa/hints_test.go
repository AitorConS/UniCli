package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestScanNanosHintsMatchesKnownFailures(t *testing.T) {
	tests := []struct {
		logs        string
		wantPattern string
	}{
		{"initdb: popen failure: Cannot allocate memory", "popen failure"},
		{"postgres: could not locate my own executable path", "could not locate my own executable"},
		{"Error loading shared library libpq.so.5: No such file or directory", "error loading shared library"},
		{"FATAL: pre-existing shared memory block (key 5432001) is still in use", "pre-existing shared memory block"},
		{"write failed: No space left on device", "no space left on device"},
	}
	for _, tt := range tests {
		hints := scanNanosHints(tt.logs)
		if len(hints) == 0 {
			t.Errorf("no hint matched for %q", tt.logs)
			continue
		}
		if hints[0].pattern != tt.wantPattern {
			t.Errorf("logs %q matched %q, want %q", tt.logs, hints[0].pattern, tt.wantPattern)
		}
	}
}

func TestScanNanosHintsPopenBeforeOOM(t *testing.T) {
	// "popen failure: Cannot allocate memory" must explain fork/exec, not OOM.
	hints := scanNanosHints("popen failure: Cannot allocate memory")
	if len(hints) == 0 || hints[0].pattern != "popen failure" {
		t.Fatalf("expected popen hint first, got %v", hints)
	}
}

func TestScanNanosHintsCleanLogs(t *testing.T) {
	if hints := scanNanosHints("database system is ready to accept connections"); len(hints) != 0 {
		t.Fatalf("expected no hints for healthy logs, got %v", hints)
	}
}

func TestHintCollectorTeesAndScans(t *testing.T) {
	var inner bytes.Buffer
	c := newHintCollector(&inner)
	if _, err := c.Write([]byte("boot ok\nerror loading shared library libssl.so.3\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inner.String(), "boot ok") {
		t.Fatal("collector did not tee to inner writer")
	}
	var hintOut bytes.Buffer
	c.PrintHints(&hintOut)
	if !strings.Contains(hintOut.String(), "shared library") {
		t.Fatalf("expected shared library hint, got %q", hintOut.String())
	}
}

func TestHintCollectorBounded(t *testing.T) {
	var inner bytes.Buffer
	c := newHintCollector(&inner)
	chunk := bytes.Repeat([]byte("x"), 64<<10)
	for i := 0; i < 10; i++ {
		if _, err := c.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if len(c.buf) > hintCollectorMax {
		t.Fatalf("retained %d bytes, cap is %d", len(c.buf), hintCollectorMax)
	}
	if inner.Len() != 10*64<<10 {
		t.Fatalf("inner writer received %d bytes, want all %d", inner.Len(), 10*64<<10)
	}
}

func TestHintCollectorRetainsTail(t *testing.T) {
	// Failure signatures appear at the END of a long log; the collector must
	// keep the most recent bytes, not the first ones.
	var inner bytes.Buffer
	c := newHintCollector(&inner)
	chunk := bytes.Repeat([]byte("startup chatter\n"), 4<<10) // 64 KiB per write
	for i := 0; i < 8; i++ {                                  // 512 KiB, twice the cap
		if _, err := c.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Write([]byte("write failed: No space left on device\n")); err != nil {
		t.Fatal(err)
	}
	var hintOut bytes.Buffer
	c.PrintHints(&hintOut)
	if !strings.Contains(hintOut.String(), "no space left on device") {
		t.Fatalf("failure signature past the retention cap was lost: %q", hintOut.String())
	}
}

func TestTailLines(t *testing.T) {
	s := "a\nb\nc\nd\n"
	if got := tailLines(s, 2); got != "c\nd" {
		t.Fatalf("tailLines = %q", got)
	}
	if got := tailLines("only", 5); got != "only" {
		t.Fatalf("tailLines short input = %q", got)
	}
}

func TestIndentLines(t *testing.T) {
	if got := indentLines("a\nb"); got != "  a\n  b" {
		t.Fatalf("indentLines = %q", got)
	}
	if got := indentLines(""); got != "" {
		t.Fatalf("indentLines empty = %q", got)
	}
}

func TestPrintNanosHintsFormat(t *testing.T) {
	var out bytes.Buffer
	printNanosHints(&out, "error loading shared library libssl.so.3")
	s := out.String()
	if !strings.Contains(s, `hint: matched "error loading shared library"`) {
		t.Fatalf("missing hint header: %q", s)
	}
	if !strings.Contains(s, "statically linked") {
		t.Fatalf("missing hint body: %q", s)
	}
	out.Reset()
	printNanosHints(&out, "all good")
	if out.Len() != 0 {
		t.Fatalf("expected no output for clean logs, got %q", out.String())
	}
}
