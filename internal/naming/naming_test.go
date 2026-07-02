package naming

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateResourceNameRejectsTraversal(t *testing.T) {
	bad := []string{
		"",
		"..",
		".",
		"../x",
		"a/b",
		`a\b`,
		"foo/../bar",
		"/abs",
		".hidden",
		"-leading-dash",
		"name with space",
		strings.Repeat("x", MaxResourceNameLen+1),
	}
	for _, name := range bad {
		if err := ValidateResourceName("network", name); err == nil {
			t.Errorf("ValidateResourceName(%q) = nil, want error", name)
		}
	}
}

func TestValidateResourceNameAcceptsGood(t *testing.T) {
	good := []string{"app", "app-backend", "db_1", "web.prod", "a", "N123", strings.Repeat("x", MaxResourceNameLen)}
	for _, name := range good {
		if err := ValidateResourceName("volume", name); err != nil {
			t.Errorf("ValidateResourceName(%q) = %v, want nil", name, err)
		}
	}
}

func TestSafeJoinStaysInRoot(t *testing.T) {
	root := filepath.FromSlash("/store/root")
	got, err := SafeJoin(root, "app")
	if err != nil {
		t.Fatalf("SafeJoin: %v", err)
	}
	if got != filepath.Join(root, "app") {
		t.Fatalf("SafeJoin = %q", got)
	}
}

func TestSafeJoinRejectsEscape(t *testing.T) {
	root := filepath.FromSlash("/store/root")
	// These would only reach SafeJoin if validation were bypassed; the guard is
	// defense in depth.
	for _, name := range []string{"../../etc", ".." + string(filepath.Separator) + "x"} {
		if _, err := SafeJoin(root, name); err == nil {
			t.Errorf("SafeJoin(%q) = nil, want error", name)
		}
	}
}
