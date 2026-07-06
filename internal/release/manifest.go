package release

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Component names published in a channel manifest.
const (
	ComponentCLI     = "cli"
	ComponentDaemon  = "daemon"
	ComponentKernel  = "kernel"
	ComponentDistro  = "distro"
	ComponentDesktop = "desktop"
)

// Manifest is the signed source of truth for a release channel. It is fetched
// from <base>/channels/<channel>.json and verified against the embedded
// minisign public key before any component is trusted.
type Manifest struct {
	Channel    string               `json:"channel"`
	Generated  string               `json:"generated,omitempty"`
	Components map[string]Component `json:"components"`
}

// Asset is one downloadable file: its URL, expected SHA-256, and size. The hash
// is authoritative because the enclosing manifest is signature-verified.
type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size,omitempty"`
}

func (a Asset) valid() bool { return a.URL != "" && a.SHA256 != "" }

// Component describes a release component and the compatibility bounds that keep
// an update from breaking a working install. Single-platform components (kernel,
// distro) carry the asset fields inline; per-platform components (cli, daemon)
// populate Platforms keyed by "<goos>-<goarch>" (e.g. "windows-amd64").
type Component struct {
	Version string `json:"version"`

	// Inline asset for single-platform components.
	URL    string `json:"url,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`

	// Per-platform assets for components that ship one binary per OS/arch.
	Platforms map[string]Asset `json:"platforms,omitempty"`

	// Files holds the named assets of a multi-file component (e.g. the kernel
	// toolset: kernel.img, boot.img, kernel-fc.img, mkfs, dump).
	Files map[string]Asset `json:"files,omitempty"`

	// Compatibility (optional, component-specific).
	MinCLI    string `json:"min_cli,omitempty"` // daemon: oldest CLI it speaks to
	Proto     int    `json:"proto,omitempty"`   // daemon: wire protocol version
	KernelVer string `json:"kernel,omitempty"`  // distro: kernel version baked in
}

// Asset resolves the download for a given platform. Per-platform components look
// up "<goos>-<goarch>"; single-platform components ignore the arguments and
// return their inline asset.
func (c Component) Asset(goos, goarch string) (Asset, error) {
	if len(c.Platforms) > 0 {
		key := goos + "-" + goarch
		a, ok := c.Platforms[key]
		if !ok {
			return Asset{}, fmt.Errorf("release: no asset for platform %q", key)
		}
		return a, nil
	}
	return Asset{URL: c.URL, SHA256: c.SHA256, Size: c.Size}, nil
}

// ParseManifest decodes and validates a channel manifest.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("release: parse manifest: %w", err)
	}
	if m.Channel == "" {
		return nil, fmt.Errorf("release: manifest missing channel")
	}
	if len(m.Components) == 0 {
		return nil, fmt.Errorf("release: manifest has no components")
	}
	for name, c := range m.Components {
		if c.Version == "" {
			return nil, fmt.Errorf("release: component %q missing version", name)
		}
		inline := Asset{URL: c.URL, SHA256: c.SHA256}.valid()
		if !inline && len(c.Platforms) == 0 && len(c.Files) == 0 {
			return nil, fmt.Errorf("release: component %q has no asset (url+sha256, platforms, or files)", name)
		}
		for plat, a := range c.Platforms {
			if !a.valid() {
				return nil, fmt.Errorf("release: component %q platform %q missing url/sha256", name, plat)
			}
		}
		for fname, a := range c.Files {
			if !a.valid() {
				return nil, fmt.Errorf("release: component %q file %q missing url/sha256", name, fname)
			}
		}
	}
	return &m, nil
}

// Component returns the named component and whether it is present.
func (m *Manifest) Component(name string) (Component, bool) {
	c, ok := m.Components[name]
	return c, ok
}

// IsNewer reports whether remote is a strictly higher semver than local.
// A leading "v" is optional on either side; malformed versions sort as 0.0.0,
// so an unknown local version always sees remote as newer.
func IsNewer(local, remote string) bool {
	return semverGT(remote, local)
}

func semverGT(a, b string) bool {
	av, bv := parseSemver(a), parseSemver(b)
	for i := range av {
		if av[i] != bv[i] {
			return av[i] > bv[i]
		}
	}
	return false
}

func parseSemver(s string) [3]int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	// Drop any pre-release / build suffix.
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	var out [3]int
	for i, p := range strings.SplitN(s, ".", 3) {
		if i >= 3 {
			break
		}
		out[i], _ = strconv.Atoi(p)
	}
	return out
}
