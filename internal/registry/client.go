package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/AitorConS/unikernel-engine/internal/image"
)

// Client pushes and pulls images to/from a registry Server.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient returns a Client targeting the registry at baseURL (e.g. "http://localhost:5000").
func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{}}
}

// Push uploads a manifest and its disk image to the registry.
func (c *Client) Push(ctx context.Context, m image.Manifest, diskPath string) error {
	manifestJSON, err := image.Marshal(m)
	if err != nil {
		return fmt.Errorf("registry push: marshal manifest: %w", err)
	}
	body, ct, err := buildMultipart(manifestJSON, diskPath)
	if err != nil {
		return fmt.Errorf("registry push: build multipart: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v2/images", body)
	if err != nil {
		return fmt.Errorf("registry push: build request: %w", err)
	}
	req.Header.Set("Content-Type", ct)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("registry push: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("registry client: close push body", "err", err)
		}
	}()
	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registry push: server returned %d: %s", resp.StatusCode, msg)
	}
	return nil
}

// Pull downloads the manifest and disk image for ref and stores them in store.
func (c *Client) Pull(ctx context.Context, ref string, store *image.Store) (image.Manifest, error) {
	m, err := c.getManifest(ctx, ref)
	if err != nil {
		return image.Manifest{}, fmt.Errorf("registry pull %s: %w", ref, err)
	}
	diskPath, err := c.getDisk(ctx, ref)
	if err != nil {
		return image.Manifest{}, fmt.Errorf("registry pull %s: %w", ref, err)
	}
	defer func() { _ = os.Remove(diskPath) }()

	if err := store.Put(m.Name, m.Tag, m, diskPath); err != nil {
		return image.Manifest{}, fmt.Errorf("registry pull %s: store: %w", ref, err)
	}
	return m, nil
}

// List returns all manifests known to the registry.
func (c *Client) List(ctx context.Context) ([]image.Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v2/images", nil)
	if err != nil {
		return nil, fmt.Errorf("registry list: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registry list: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("registry client: close list body", "err", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry list: server returned %d", resp.StatusCode)
	}
	var out []image.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("registry list: decode: %w", err)
	}
	return out, nil
}

func (c *Client) getManifest(ctx context.Context, ref string) (image.Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/v2/images/"+ref, nil)
	if err != nil {
		return image.Manifest{}, fmt.Errorf("get manifest: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return image.Manifest{}, fmt.Errorf("get manifest: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("registry client: close manifest body", "err", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return image.Manifest{}, fmt.Errorf("get manifest: server returned %d", resp.StatusCode)
	}
	var m image.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return image.Manifest{}, fmt.Errorf("get manifest: decode: %w", err)
	}
	return m, nil
}

func (c *Client) getDisk(ctx context.Context, ref string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/v2/images/"+ref+"/disk", nil)
	if err != nil {
		return "", fmt.Errorf("get disk: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("get disk: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("registry client: close disk body", "err", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get disk: server returned %d", resp.StatusCode)
	}
	f, err := os.CreateTemp("", "uni-pull-*.img")
	if err != nil {
		return "", fmt.Errorf("get disk: create temp: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			_ = err // best effort
		}
	}()
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("get disk: write temp: %w", err)
	}
	return f.Name(), nil
}

func buildMultipart(manifestJSON []byte, diskPath string) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if err := w.WriteField("manifest", string(manifestJSON)); err != nil {
		return nil, "", fmt.Errorf("write manifest field: %w", err)
	}
	fw, err := w.CreateFormFile("disk", "disk.img")
	if err != nil {
		return nil, "", fmt.Errorf("create disk field: %w", err)
	}
	f, err := os.Open(diskPath)
	if err != nil {
		return nil, "", fmt.Errorf("open disk %s: %w", diskPath, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			_ = err // best effort
		}
	}()
	if _, err := io.Copy(fw, f); err != nil {
		return nil, "", fmt.Errorf("copy disk: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart writer: %w", err)
	}
	return &buf, w.FormDataContentType(), nil
}
