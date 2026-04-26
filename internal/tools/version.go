package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	versionFileName  = "kernel-version.txt"
	versionRemoteURL = "https://github.com/AitorConS/UniCLi/releases/download/latest/kernel-version.txt"
)

// LocalVersion returns the version string stored in toolsDir/kernel-version.txt.
// Returns "(unknown)" if the file is absent or unreadable.
func LocalVersion(toolsDir string) string {
	data, err := os.ReadFile(filepath.Join(toolsDir, versionFileName))
	if err != nil {
		return "(unknown)"
	}
	return strings.TrimSpace(string(data))
}

// RemoteVersion fetches the latest published kernel version from GitHub releases.
// Returns an error if the network is unreachable or the release has no version file.
func RemoteVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionRemoteURL, nil)
	if err != nil {
		return "", fmt.Errorf("tools: build version request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("tools: fetch remote version: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tools: remote version fetch returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", fmt.Errorf("tools: read remote version: %w", err)
	}
	return strings.TrimSpace(string(body)), nil
}

// SaveLocalVersion writes version to toolsDir/kernel-version.txt.
func SaveLocalVersion(toolsDir, version string) error {
	path := filepath.Join(toolsDir, versionFileName)
	if err := os.WriteFile(path, []byte(version+"\n"), 0o644); err != nil {
		return fmt.Errorf("tools: save kernel version: %w", err)
	}
	return nil
}

// ClearCachedTools deletes the three kernel artifacts and the version file from
// toolsDir so that the next ResolveMkfs call re-downloads them.
func ClearCachedTools(toolsDir string) error {
	for _, name := range []string{"mkfs", "kernel.img", "boot.img", versionFileName} {
		if err := os.Remove(filepath.Join(toolsDir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("tools: clear %s: %w", name, err)
		}
	}
	return nil
}

// ToolsExist returns true when all three kernel artifacts are present in toolsDir.
func ToolsExist(toolsDir string) bool {
	for _, name := range []string{"mkfs", "kernel.img", "boot.img"} {
		if _, err := os.Stat(filepath.Join(toolsDir, name)); os.IsNotExist(err) {
			return false
		}
	}
	return true
}
