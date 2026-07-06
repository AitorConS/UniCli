// Package release is the single, shared path for discovering and downloading
// Jerboa's distributable components (kernel, daemon, CLI, distro, desktop).
//
// It replaces the four ad-hoc downloaders scattered across the tree with one
// manifest-driven model: a minisign-signed channel manifest is the source of
// truth, and every artifact is verified by the SHA-256 recorded in that signed
// manifest. Because the manifest's signature transitively covers each artifact
// hash, individual artifacts need no separate signature and can be streamed and
// verified without buffering large files in memory.
package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/AitorConS/jerboa/internal/httpclient"
)

// DefaultBaseURL is the public read origin backed by the R2 bucket.
const DefaultBaseURL = "https://releases.jerboa.dev"

// Channel names.
const (
	ChannelStable = "stable"
	ChannelBeta   = "beta"
)

// Client fetches and verifies release manifests and artifacts.
type Client struct {
	// BaseURL is the release origin. Empty means DefaultBaseURL.
	BaseURL string
	// Pub is the minisign public key the manifest is verified against.
	Pub PublicKey
	// HTTP is the client used for downloads. Nil means httpclient.Default.
	HTTP *http.Client
}

// NewClient builds a Client from a base URL and an embedded minisign public key.
// An empty baseURL falls back to DefaultBaseURL.
func NewClient(baseURL, publicKey string) (*Client, error) {
	pk, err := ParsePublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Pub: pk}, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return httpclient.Default
}

func (c *Client) base() string {
	if c.BaseURL == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

// FetchManifest downloads channel/<channel>.json and its .minisig, verifies the
// signature against the embedded public key, and returns the parsed manifest.
func (c *Client) FetchManifest(ctx context.Context, channel string) (*Manifest, error) {
	manifestURL := c.base() + "/channels/" + channel + ".json"
	sigURL := manifestURL + ".minisig"

	manifestBytes, err := c.get(ctx, manifestURL)
	if err != nil {
		return nil, fmt.Errorf("release: fetch manifest: %w", err)
	}
	sigBytes, err := c.get(ctx, sigURL)
	if err != nil {
		return nil, fmt.Errorf("release: fetch manifest signature: %w", err)
	}
	if err := c.Pub.Verify(manifestBytes, sigBytes); err != nil {
		return nil, err
	}
	return ParseManifest(manifestBytes)
}

// DownloadArtifact streams a.URL to dest, verifying length and SHA-256 against
// the (already signature-verified) manifest asset. The write is atomic: it lands
// in dest.tmp and is renamed into place only after verification, so a crash or
// hash mismatch never leaves a partial artifact at dest.
func (c *Client) DownloadArtifact(ctx context.Context, a Asset, dest string) error {
	if a.SHA256 == "" {
		return fmt.Errorf("release: refusing to download %s with no sha256", a.URL)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("release: create dest dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return fmt.Errorf("release: build request: %w", err)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("release: download %s: %w", a.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("release: download %s: HTTP %d", a.URL, resp.StatusCode)
	}

	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("release: create temp file: %w", err)
	}
	// Ensure the temp file never lingers on any error path.
	cleanup := true
	defer func() {
		if cleanup {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()

	h := sha256.New()
	reader := io.TeeReader(resp.Body, h)
	if a.Size > 0 {
		// Cap the stream so a misbehaving/compromised origin cannot write an
		// unbounded amount to disk before the hash check fails. +1 so an
		// oversized body trips the size mismatch below instead of being
		// silently truncated to exactly a.Size bytes.
		reader = io.LimitReader(reader, a.Size+1)
	}
	written, err := io.Copy(f, reader)
	if err != nil {
		return fmt.Errorf("release: write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("release: close %s: %w", tmp, err)
	}

	if a.Size > 0 && written != a.Size {
		return fmt.Errorf("release: %s size mismatch: got %d, want %d", a.URL, written, a.Size)
	}
	got := hex.EncodeToString(h.Sum(nil))
	want := strings.ToLower(strings.TrimSpace(a.SHA256))
	if got != want {
		return fmt.Errorf("release: %s sha256 mismatch: got %s, want %s", a.URL, got, want)
	}

	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("release: install %s: %w", dest, err)
	}
	cleanup = false
	return nil
}

// get performs a GET and returns the body, capping the response size so a
// misbehaving origin cannot exhaust memory on a small metadata fetch.
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("release: build request: %w", err)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("release: get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	const maxMeta = 1 << 20 // 1 MiB is plenty for a manifest or signature.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMeta))
	if err != nil {
		return nil, fmt.Errorf("release: read %s: %w", url, err)
	}
	return body, nil
}
