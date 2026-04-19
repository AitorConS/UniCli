package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/AitorConS/unikernel-engine/internal/image"
)

const mkfsDownloadURL = "https://github.com/AitorConS/unikernel-engine/releases/download/latest/mkfs-linux-amd64"

// ResolveMkfs returns an image.MkfsFunc ready to invoke.
//
// If override is non-empty it is used as-is (advanced users with a local mkfs).
// Otherwise mkfs-linux-amd64 is downloaded to toolsDir on first use and cached.
// On Windows, mkfs is automatically invoked through WSL2.
func ResolveMkfs(ctx context.Context, toolsDir, override string) (image.MkfsFunc, error) {
	if override != "" {
		return directFunc(override), nil
	}

	mkfsPath := filepath.Join(toolsDir, "mkfs")
	if _, err := os.Stat(mkfsPath); os.IsNotExist(err) {
		if err := downloadMkfs(ctx, mkfsPath); err != nil {
			return nil, err
		}
	}

	if runtime.GOOS == "windows" {
		return wslFunc(mkfsPath)
	}
	return directFunc(mkfsPath), nil
}

// directFunc returns an image.MkfsFunc that calls mkfsBin directly.
func directFunc(mkfsBin string) image.MkfsFunc {
	return func(ctx context.Context, imgPath, binaryPath string) *exec.Cmd {
		return exec.CommandContext(ctx, mkfsBin, imgPath, binaryPath)
	}
}

func downloadMkfs(ctx context.Context, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("tools: create dir: %w", err)
	}
	fmt.Printf("mkfs not found in cache — downloading from GitHub releases...\n")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mkfsDownloadURL, nil)
	if err != nil {
		return fmt.Errorf("tools: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("tools: download mkfs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tools: download mkfs: server returned %d", resp.StatusCode)
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("tools: create mkfs file: %w", err)
	}
	defer func() { _ = f.Close() }()

	size, err := io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("tools: write mkfs: %w", err)
	}
	fmt.Printf("mkfs downloaded (%.1f MB) → %s\n", float64(size)/(1<<20), dest)
	return nil
}
