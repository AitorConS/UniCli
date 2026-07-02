// Package naming validates user-supplied resource names (networks, volumes)
// before they are used to build filesystem paths. Names arrive over the daemon
// RPC surface and are joined onto a store root, so an unvalidated name like
// "../x" would let a client read, write, or delete outside the store.
package naming

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// MaxResourceNameLen bounds a resource name. It is generous enough for
// descriptive names while keeping derived artifacts (directories, labels) sane.
const MaxResourceNameLen = 64

// resourceNameRe requires the name to start with an alphanumeric character and
// contain only alphanumerics, underscore, dot, and hyphen. Because the first
// character must be alphanumeric, the reserved traversal names "." and ".." are
// rejected, and there is no way to embed a path separator.
var resourceNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ValidateResourceName reports whether name is safe to use as a single path
// component under a store root. kind is used only for the error message
// (e.g. "network", "volume").
func ValidateResourceName(kind, name string) error {
	if name == "" {
		return fmt.Errorf("%s name must not be empty", kind)
	}
	if len(name) > MaxResourceNameLen {
		return fmt.Errorf("%s name %q is too long (max %d characters)", kind, name, MaxResourceNameLen)
	}
	// Reject separators explicitly for a clearer message than the regex miss.
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("%s name %q must not contain path separators or %q", kind, name, "..")
	}
	if !resourceNameRe.MatchString(name) {
		return fmt.Errorf("%s name %q is invalid (allowed: letters, digits, and _.- ; must start with a letter or digit)", kind, name)
	}
	return nil
}

// SafeJoin joins name onto root and verifies the result stays inside root, as a
// defense-in-depth guard against traversal even if the name slipped validation.
// It returns an error rather than a path when the join would escape.
func SafeJoin(root, name string) (string, error) {
	joined := filepath.Join(root, name)
	rel, err := filepath.Rel(root, joined)
	if err != nil {
		return "", fmt.Errorf("resolve path for %q: %w", name, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path for %q escapes the store root", name)
	}
	return joined, nil
}
