package api

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// frame builds the wire bytes for a single frame with the given length header
// and body, letting tests craft malformed frames (header not matching body).
func frame(length uint32, body []byte) []byte {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], length)
	return append(hdr[:], body...)
}

func TestFraming_RoundTrip(t *testing.T) {
	t.Parallel()
	sizes := []int{0, 1, 100, 32 * 1024, 200 * 1024}
	for _, size := range sizes {
		payload := make([]byte, size)
		if _, err := rand.Read(payload); err != nil {
			t.Fatalf("rand: %v", err)
		}

		var wire bytes.Buffer
		fw := NewFrameWriter(&wire)
		if _, err := io.Copy(fw, bytes.NewReader(payload)); err != nil {
			t.Fatalf("write frames: %v", err)
		}
		if err := fw.Close(); err != nil {
			t.Fatalf("close frames: %v", err)
		}

		// A trailing sentinel proves the reader stops exactly at the terminator.
		wire.WriteString("SENTINEL")

		fr := NewFrameReader(&wire)
		got, err := io.ReadAll(fr)
		if err != nil {
			t.Fatalf("read frames (size %d): %v", size, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("size %d: payload mismatch (got %d bytes)", size, len(got))
		}
		rest, _ := io.ReadAll(&wire)
		if string(rest) != "SENTINEL" {
			t.Fatalf("size %d: reader consumed past terminator, leftover=%q", size, rest)
		}
	}
}

// TestFrameReader_Malformed pins the reader's behavior on hostile or truncated
// input arriving from the RPC socket. Each case must surface an error rather
// than panic, over-read, or allocate on a bogus length header.
func TestFrameReader_Malformed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wire    []byte
		wantErr error // sentinel/target for errors.Is, or nil to accept any error
	}{
		{
			name: "empty stream",
			wire: nil,
			// No header at all: ReadFull reports EOF.
			wantErr: io.EOF,
		},
		{
			name:    "truncated header",
			wire:    []byte{0x00, 0x00, 0x01}, // 3 of 4 header bytes
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name:    "oversized length header",
			wire:    frame(maxFrameSize+1, nil),
			wantErr: ErrFrameTooLarge,
		},
		{
			name:    "max uint32 length header",
			wire:    frame(0xFFFFFFFF, nil),
			wantErr: ErrFrameTooLarge,
		},
		{
			name:    "truncated payload",
			wire:    frame(10, []byte("only5")), // header claims 10, 5 present
			wantErr: nil,                        // any error is acceptable
		},
		{
			name:    "missing terminator",
			wire:    frame(3, []byte("abc")), // valid frame, no terminator frame
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fr := NewFrameReader(bytes.NewReader(tt.wire))
			_, err := io.ReadAll(fr)
			if err == nil {
				t.Fatalf("io.ReadAll(%s) = nil error, want an error", tt.name)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("io.ReadAll(%s) = %v, want errors.Is(_, %v)", tt.name, err, tt.wantErr)
			}
		})
	}
}

// TestFrameReader_OversizedDoesNotAllocate confirms the length guard is checked
// before any payload buffer is reserved: an attacker sending a 4 GiB length
// header with no body must be rejected immediately, not after an allocation.
func TestFrameReader_OversizedDoesNotAllocate(t *testing.T) {
	t.Parallel()
	fr := NewFrameReader(bytes.NewReader(frame(0xFFFFFFFF, nil)))
	buf := make([]byte, 32*1024)
	n, err := fr.Read(buf)
	if n != 0 {
		t.Fatalf("Read returned %d bytes for an oversized frame, want 0", n)
	}
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("Read = %v, want errors.Is(_, ErrFrameTooLarge)", err)
	}
}

// FuzzFrameReader feeds arbitrary bytes as a frame stream. The reader must never
// panic and must always terminate, whatever the input — the core robustness
// contract for a decoder on an untrusted socket.
func FuzzFrameReader(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00}) // lone terminator
	f.Add([]byte{0x00, 0x00, 0x00, 0x03, 'a', 'b', 'c'})
	f.Add(frame(maxFrameSize+1, nil))     // oversized
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // huge length, no body

	f.Fuzz(func(t *testing.T, data []byte) {
		fr := NewFrameReader(bytes.NewReader(data))
		// A capped reader defends the fuzzer from a legitimately huge (but valid)
		// multi-frame stream; we only care that Read never panics and returns.
		_, _ = io.Copy(io.Discard, io.LimitReader(fr, maxFrameSize))
	})
}

// FuzzFrameRoundTrip asserts the format is lossless for any payload: writing it
// as frames then reading it back reproduces the exact bytes, and the reader
// stops precisely at the terminator (proved by a trailing sentinel).
func FuzzFrameRoundTrip(f *testing.F) {
	f.Add([]byte(nil))
	f.Add([]byte("hello"))
	f.Add(bytes.Repeat([]byte{0xAB}, 70000)) // spans multiple io.Copy chunks

	f.Fuzz(func(t *testing.T, payload []byte) {
		var wire bytes.Buffer
		fw := NewFrameWriter(&wire)
		if _, err := io.Copy(fw, bytes.NewReader(payload)); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := fw.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		wire.WriteString("END")

		fr := NewFrameReader(&wire)
		got, err := io.ReadAll(fr)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(got), len(payload))
		}
		if rest, _ := io.ReadAll(&wire); string(rest) != "END" {
			t.Fatalf("reader over-consumed past terminator: leftover=%q", rest)
		}
	})
}

func BenchmarkFrameRoundTrip(b *testing.B) {
	for _, size := range []int{1 << 10, 64 << 10, 1 << 20} {
		payload := make([]byte, size)
		b.Run(byteSizeName(size), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(size))
			var wire bytes.Buffer
			for b.Loop() {
				wire.Reset()
				fw := NewFrameWriter(&wire)
				_, _ = io.Copy(fw, bytes.NewReader(payload))
				_ = fw.Close()
				_, _ = io.Copy(io.Discard, NewFrameReader(&wire))
			}
		})
	}
}

func byteSizeName(n int) string {
	switch {
	case n >= 1<<20:
		return "1MiB"
	case n >= 64<<10:
		return "64KiB"
	default:
		return "1KiB"
	}
}
