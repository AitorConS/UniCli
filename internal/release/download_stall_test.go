package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// BUG-001: a 30s total download deadline aborted large, healthy transfers.
// DownloadArtifact now streams via httpclient.Streaming (no total deadline) and
// only aborts on a real stall. The "no total deadline" property and the
// long-but-progressing case are covered by the httpclient unit tests
// (TestStreamingClient_HasNoTotalDeadline, TestCopyWithStall_LongButSteady...);
// here we verify the end-to-end download path — streaming client, stall
// watchdog, hash + size verification, atomic rename — actually works.

func TestDownloadArtifact_VerifiesAndInstalls(t *testing.T) {
	body := make([]byte, 4000)
	for i := range body {
		body[i] = byte('A' + i%26)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	sum := sha256.Sum256(body)
	c := &Client{}
	dest := filepath.Join(t.TempDir(), "artifact.bin")
	a := Asset{URL: srv.URL + "/artifact", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))}

	require.NoError(t, c.DownloadArtifact(context.Background(), a, dest))
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, body, got)
}

func TestDownloadArtifact_HashMismatchRejected(t *testing.T) {
	body := []byte("some artifact bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := &Client{}
	dest := filepath.Join(t.TempDir(), "artifact.bin")
	// A hash that does not match the body must be rejected and leave nothing behind.
	a := Asset{URL: srv.URL + "/artifact", SHA256: "00" + hex.EncodeToString(sha256.New().Sum(nil))[2:]}

	err := c.DownloadArtifact(context.Background(), a, dest)
	require.Error(t, err)
	require.Contains(t, err.Error(), "sha256 mismatch")
	_, statErr := os.Stat(dest)
	require.True(t, os.IsNotExist(statErr))
}

func TestDownloadArtifact_StalledTransferAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("partial"))
		if fl != nil {
			fl.Flush()
		}
		// Hang until the client cancels the request (the stall watchdog fires),
		// then return — so no server goroutine leaks past the test.
		<-r.Context().Done()
	}))
	defer srv.Close()

	orig := StallTimeout
	StallTimeout = 100 * time.Millisecond
	defer func() { StallTimeout = orig }()

	c := &Client{}
	dest := filepath.Join(t.TempDir(), "artifact.bin")
	a := Asset{URL: srv.URL + "/artifact", SHA256: "deadbeef"} // non-empty; copy fails before the hash check

	err := c.DownloadArtifact(context.Background(), a, dest)
	require.Error(t, err)
	require.Contains(t, err.Error(), "stalled")
	// The atomic write must leave no partial artifact behind.
	_, statErr := os.Stat(dest)
	require.True(t, os.IsNotExist(statErr), "a failed download must not leave a partial file at dest")
}
