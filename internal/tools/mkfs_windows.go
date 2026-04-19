//go:build windows

package tools

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AitorConS/unikernel-engine/internal/image"
)

// wslFunc returns an image.MkfsFunc that invokes mkfs via WSL2.
func wslFunc(mkfsPath string) (image.MkfsFunc, error) {
	if err := checkWSL2(); err != nil {
		return nil, err
	}
	wslMkfs := windowsToWSLPath(mkfsPath)
	return func(ctx context.Context, imgPath, binaryPath string) *exec.Cmd {
		return exec.CommandContext(ctx, "wsl", "--",
			wslMkfs,
			windowsToWSLPath(imgPath),
			windowsToWSLPath(binaryPath),
		)
	}, nil
}

// checkWSL2 verifies that WSL2 is available and functional.
func checkWSL2() error {
	cmd := exec.Command("wsl", "--", "echo", "ok")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(
			"WSL2 is required to build unikernel images on Windows.\n"+
				"Install it with: wsl --install\n"+
				"See: https://learn.microsoft.com/windows/wsl/install\n"+
				"(underlying error: %w)", err)
	}
	return nil
}

// windowsToWSLPath converts a Windows absolute path to its WSL2 mount path.
// Example: C:\Users\foo\bar → /mnt/c/Users/foo/bar
func windowsToWSLPath(p string) string {
	p = filepath.ToSlash(p)
	if len(p) >= 3 && p[1] == ':' && p[2] == '/' {
		drive := strings.ToLower(string(p[0]))
		return "/mnt/" + drive + p[2:]
	}
	return p
}
