package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ResolveDump returns the path to the dump binary, downloading it from the
// kernel release if it is not already present in toolsDir.
// If override is non-empty it is returned as-is without any checks.
func ResolveDump(ctx context.Context, toolsDir, override string) (string, error) {
	if override != "" {
		return override, nil
	}

	dumpPath := filepath.Join(toolsDir, "dump")
	if runtime.GOOS == "windows" {
		dumpPath += ".exe"
	}

	if _, err := os.Stat(dumpPath); err == nil {
		return dumpPath, nil
	}

	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return "", fmt.Errorf("tools: create tools dir: %w", err)
	}

	// Download dump from the latest kernel release.
	if err := downloadArtifact(ctx, ArtifactURL("latest", "dump-linux-amd64"), dumpPath); err != nil {
		return "", fmt.Errorf("tools: download dump: %w", err)
	}
	return dumpPath, nil
}
