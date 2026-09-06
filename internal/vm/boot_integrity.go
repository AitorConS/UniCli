//go:build linux

package vm

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

func verifyBootCopy(path, digest string) error {
	if digest == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("verify boot image: %w", err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("verify boot image: %w", err)
	}
	if fmt.Sprintf("sha256:%x", h.Sum(nil)) != digest {
		return fmt.Errorf("boot image integrity mismatch")
	}
	return nil
}
