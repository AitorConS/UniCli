//go:build !windows

package tools

import (
	"fmt"

	"github.com/AitorConS/unikernel-engine/internal/image"
)

// wslFunc is unreachable on non-Windows platforms.
func wslFunc(_, _, _ string) (image.MkfsFunc, error) {
	return nil, fmt.Errorf("wslFunc called on non-Windows platform (bug)")
}
