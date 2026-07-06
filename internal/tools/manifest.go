package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AitorConS/jerboa/internal/release"
)

// kernelFileLocalNames maps the manifest's kernel file keys to their on-disk
// names in the tools dir (mkfs/dump drop their -linux-amd64 suffix, matching
// DownloadVersion's legacy layout).
var kernelFileLocalNames = map[string]string{
	"kernel.img":    "kernel.img",
	"boot.img":      "boot.img",
	"kernel-fc.img": fcKernelLocalName,
	"mkfs":          "mkfs",
	"dump":          "dump",
}

// optionalKernelFiles are absent from older releases and must not fail a fetch.
var optionalKernelFiles = map[string]bool{"dump": true, "kernel-fc.img": true}

// KernelComponentFromManifest fetches and verifies the signed channel manifest
// and returns its kernel component. Errors (including no published manifest yet)
// let callers fall back to the legacy GitHub path during the migration.
func KernelComponentFromManifest(ctx context.Context, cl *release.Client, channel string) (release.Component, error) {
	m, err := cl.FetchManifest(ctx, channel)
	if err != nil {
		return release.Component{}, err
	}
	k, ok := m.Component(release.ComponentKernel)
	if !ok {
		return release.Component{}, fmt.Errorf("tools: manifest has no kernel component")
	}
	return k, nil
}

// DownloadKernelFromManifest downloads every kernel file named by the (already
// signature-verified) manifest component into toolsDir and records the version.
// Each artifact is SHA-256 verified against the signed manifest.
func DownloadKernelFromManifest(ctx context.Context, cl *release.Client, toolsDir string, k release.Component) error {
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return fmt.Errorf("tools: create tools dir: %w", err)
	}
	for key, local := range kernelFileLocalNames {
		a, ok := k.Files[key]
		if !ok {
			if optionalKernelFiles[key] {
				continue
			}
			return fmt.Errorf("tools: kernel manifest missing required file %q", key)
		}
		if err := cl.DownloadArtifact(ctx, a, filepath.Join(toolsDir, local)); err != nil {
			return fmt.Errorf("tools: download kernel %s: %w", key, err)
		}
	}
	return SaveLocalVersion(toolsDir, k.Version)
}
