package image

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// BuildConfig holds the parameters for building a unikernel image.
type BuildConfig struct {
	// Name is the image name (e.g. "hello").
	Name string
	// Tag is the image tag (default "latest" if empty).
	Tag string
	// BinaryPath is the path to the static ELF binary to package.
	BinaryPath string
	// MkfsBin is the path to the mkfs tool (kernel/output/tools/bin/mkfs).
	MkfsBin string
	// Memory is the default VM memory string (e.g. "256M").
	Memory string
	// CPUs is the default number of virtual CPUs.
	CPUs int
}

// Builder produces unikernel images from ELF binaries and stores them.
type Builder struct {
	store *Store
}

// NewBuilder returns a Builder that stores images in store.
func NewBuilder(store *Store) *Builder {
	return &Builder{store: store}
}

// Build packages binaryPath into a disk image and registers it in the store.
func (b *Builder) Build(ctx context.Context, cfg BuildConfig) (Manifest, error) {
	if err := validateBuildConfig(cfg); err != nil {
		return Manifest{}, fmt.Errorf("build: %w", err)
	}
	if cfg.Tag == "" {
		cfg.Tag = "latest"
	}
	if cfg.Memory == "" {
		cfg.Memory = "256M"
	}
	if cfg.CPUs == 0 {
		cfg.CPUs = 1
	}
	if err := checkELF(cfg.BinaryPath); err != nil {
		return Manifest{}, fmt.Errorf("build: %w", err)
	}

	tmp, err := os.CreateTemp("", "uni-build-*.img")
	if err != nil {
		return Manifest{}, fmt.Errorf("build: create temp image: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return Manifest{}, fmt.Errorf("build: close temp: %w", err)
	}
	defer func() { _ = os.Remove(tmpPath) }()

	if err := runMkfs(ctx, cfg.MkfsBin, tmpPath, cfg.BinaryPath); err != nil {
		return Manifest{}, fmt.Errorf("build: %w", err)
	}

	stat, err := os.Stat(tmpPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("build: stat image: %w", err)
	}
	digest, err := fileSHA256(tmpPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("build: %w", err)
	}

	m := Manifest{
		SchemaVersion: SchemaVersion,
		Name:          cfg.Name,
		Tag:           cfg.Tag,
		Created:       time.Now().UTC(),
		Config: Config{
			Memory: cfg.Memory,
			CPUs:   cfg.CPUs,
		},
		DiskDigest: digest,
		DiskSize:   stat.Size(),
	}

	if err := b.store.Put(cfg.Name, cfg.Tag, m, tmpPath); err != nil {
		return Manifest{}, fmt.Errorf("build: store: %w", err)
	}
	return m, nil
}

func validateBuildConfig(cfg BuildConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("name is required")
	}
	if cfg.BinaryPath == "" {
		return fmt.Errorf("binary path is required")
	}
	if cfg.MkfsBin == "" {
		return fmt.Errorf("mkfs binary path is required")
	}
	return nil
}

func checkELF(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open binary %s: %w", path, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			_ = err // best effort
		}
	}()
	magic := make([]byte, 4)
	if _, err := f.Read(magic); err != nil {
		return fmt.Errorf("read binary %s: %w", path, err)
	}
	if magic[0] != 0x7f || magic[1] != 'E' || magic[2] != 'L' || magic[3] != 'F' {
		return fmt.Errorf("%s is not an ELF binary", path)
	}
	return nil
}

func runMkfs(ctx context.Context, mkfsBin, imgPath, binaryPath string) error {
	cmd := exec.CommandContext(ctx, mkfsBin, imgPath, binaryPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mkfs %s: %w", mkfsBin, err)
	}
	return nil
}
