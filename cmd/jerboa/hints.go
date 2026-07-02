package main

import (
	"fmt"
	"io"
	"strings"
)

// nanosHint maps a known guest failure message to an explanation of what it
// means in a unikernel and how to fix it. The Nanos kernel is single-process
// and its error strings are terse; matching them here turns "google the panic"
// into an inline explanation.
type nanosHint struct {
	// pattern is matched as a case-insensitive substring of the serial output.
	pattern string
	// hint explains the cause and the fix, shown to the user on stderr.
	hint string
}

// nanosHintTable lists the known failure signatures, most specific first: the
// first pattern that matches a given log wins over later, more generic ones
// (e.g. "popen failure: Cannot allocate memory" must explain fork/exec, not OOM).
var nanosHintTable = []nanosHint{
	{
		pattern: "popen failure",
		hint: "The program tried to launch a child process (fork/exec). Unikernels are single-process:\n" +
			"Nanos has no fork or exec, so anything that shells out (initdb, entrypoint scripts,\n" +
			"process managers) cannot run. Use a package that ships the pre-computed result\n" +
			"(e.g. eyberg/postgresql includes an initialized data directory) or a program that\n" +
			"does the work in-process.",
	},
	{
		pattern: "unimplemented syscall",
		hint: "The program used a Linux syscall Nanos does not implement — most often process\n" +
			"management (fork/exec/clone) or an exotic ioctl. Unikernels run exactly one process;\n" +
			"check whether the program can be configured not to spawn helpers or daemonize.",
	},
	{
		pattern: "could not locate my own executable",
		hint: "The program looks up its own path (/proc/self/exe) to find its install prefix, but it\n" +
			"was placed at a different path than it expects. Set [program] path in unikernel.toml\n" +
			"to the FULL in-image path (e.g. /usr/local/pgsql/bin/postgres), not a bare name that\n" +
			"may match a same-named launcher stub at the image root.",
	},
	{
		pattern: "error loading shared library",
		hint: "A shared library the program needs is missing from the image. Add the package that\n" +
			"ships it (--pkg / [build] pkgs in unikernel.toml) or use a statically linked binary.\n" +
			"'jerboa build' preflight reports these before booting — check its output.",
	},
	{
		pattern: "cannot open shared object file",
		hint: "A shared library the program needs is missing from the image. Add the package that\n" +
			"ships it (--pkg / [build] pkgs in unikernel.toml) or use a statically linked binary.",
	},
	{
		pattern: "pre-existing shared memory block",
		hint: "PostgreSQL found a stale postmaster.pid from an ungraceful shutdown (the previous VM\n" +
			"was killed instead of stopped). Always stop database VMs with 'jerboa stop' so they\n" +
			"checkpoint and clear their lock; re-seed the volume to clear a stale lock.",
	},
	{
		pattern: "out of memory",
		hint: "The guest ran out of RAM. Raise the VM memory with 'jerboa run --memory 512M' or bake\n" +
			"a bigger default into the image ([run] memory in unikernel.toml).",
	},
	{
		pattern: "failed to allocate",
		hint: "The guest is likely short on RAM. Raise the VM memory with 'jerboa run --memory 512M'\n" +
			"or bake a bigger default into the image ([run] memory in unikernel.toml).",
	},
	{
		pattern: "no space left on device",
		hint: "The root image filesystem is full. Reserve free space for runtime writes with\n" +
			"disk_size in unikernel.toml (e.g. disk_size = \"1G\") — the image is sized to its\n" +
			"contents by default, leaving no room for logs, temp files, or database writes.",
	},
	{
		pattern: "read-only file system",
		hint: "The program wrote to a path that does not exist in the image. Declare writable\n" +
			"directories with [build] dirs in unikernel.toml, or mount a volume there (-v).",
	},
}

// scanNanosHints returns the hints whose pattern appears in logs, in table
// order, at most once each.
func scanNanosHints(logs string) []nanosHint {
	lower := strings.ToLower(logs)
	var matched []nanosHint
	for _, h := range nanosHintTable {
		if strings.Contains(lower, strings.ToLower(h.pattern)) {
			matched = append(matched, h)
		}
	}
	return matched
}

// printNanosHints scans logs for known failure signatures and prints an
// explanation block for each match. No output when nothing matches.
func printNanosHints(w io.Writer, logs string) {
	for _, h := range scanNanosHints(logs) {
		fmt.Fprintf(w, "\nhint: matched %q\n", h.pattern)
		for _, line := range strings.Split(h.hint, "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
}

// hintCollector is an io.Writer that tees the stream to an inner writer while
// retaining a bounded copy for hint scanning once the stream ends. Retention is
// capped so a chatty guest cannot grow memory unboundedly; the daemon's own log
// ring buffer is similarly bounded.
type hintCollector struct {
	inner io.Writer
	buf   strings.Builder
}

const hintCollectorMax = 256 << 10 // 256 KiB of retained serial output

func newHintCollector(inner io.Writer) *hintCollector {
	return &hintCollector{inner: inner}
}

func (h *hintCollector) Write(p []byte) (int, error) {
	if h.buf.Len() < hintCollectorMax {
		room := hintCollectorMax - h.buf.Len()
		if len(p) < room {
			room = len(p)
		}
		h.buf.Write(p[:room])
	}
	return h.inner.Write(p) //nolint:wrapcheck // io.Writer pass-through: callers need the inner writer's error as-is
}

// PrintHints scans the retained output and prints matched hints to w.
func (h *hintCollector) PrintHints(w io.Writer) {
	printNanosHints(w, h.buf.String())
}
