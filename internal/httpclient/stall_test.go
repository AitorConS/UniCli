package httpclient

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// BUG-001: large downloads were killed by a 30s TOTAL deadline. The streaming
// client must set no total deadline, and CopyWithStall must abort only on an
// actual stall, never on a long-but-progressing transfer.

func TestStreamingClient_HasNoTotalDeadline(t *testing.T) {
	require.Zero(t, Streaming.Timeout, "the streaming client must not set a total request deadline")
	require.NotNil(t, Streaming.Transport, "the streaming client needs a transport with granular timeouts")
}

// steadyReader yields chunks bytes at a time with a delay before each, then EOF.
// Each gap is short, but the cumulative time can far exceed a single idle window.
type steadyReader struct {
	chunk []byte
	count int
	delay time.Duration
	sent  int
}

func (r *steadyReader) Read(p []byte) (int, error) {
	if r.sent >= r.count {
		return 0, io.EOF
	}
	time.Sleep(r.delay)
	n := copy(p, r.chunk)
	r.sent++
	return n, nil
}

func TestCopyWithStall_LongButSteadyTransferCompletes(t *testing.T) {
	// 10 chunks × 20ms = ~200ms total, well over the 60ms stall window, but each
	// gap (20ms) is under it — so a healthy long transfer must complete.
	r := &steadyReader{chunk: bytes.Repeat([]byte("y"), 10), count: 10, delay: 20 * time.Millisecond}
	var buf bytes.Buffer
	n, err := CopyWithStall(&buf, r, 60*time.Millisecond, func() {})
	require.NoError(t, err)
	require.Equal(t, int64(100), n)
	require.Equal(t, 100, buf.Len())
}

// stallingReader delivers stallAt bytes, then blocks until unblocked is closed
// (simulating a dead connection whose Read only returns when the request is
// canceled), returning an error thereafter.
type stallingReader struct {
	data      []byte
	pos       int
	unblocked chan struct{}
}

func (r *stallingReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		<-r.unblocked
		return 0, context.Canceled
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func TestCopyWithStall_AbortsOnStall(t *testing.T) {
	r := &stallingReader{data: bytes.Repeat([]byte("x"), 100), unblocked: make(chan struct{})}
	// cancel unblocks the stuck Read, mirroring how canceling the request context
	// aborts a Read stuck on a silent socket.
	cancel := func() { close(r.unblocked) }

	var buf bytes.Buffer
	n, err := CopyWithStall(&buf, r, 40*time.Millisecond, cancel)
	require.Error(t, err)
	var se *StallError
	require.ErrorAs(t, err, &se, "a stalled transfer must return a *StallError")
	require.Equal(t, int64(100), n, "bytes received before the stall are still copied")
}
