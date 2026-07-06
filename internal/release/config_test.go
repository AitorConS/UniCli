package release

import "testing"

func TestEmbeddedPublicKeyParses(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatalf("Default(): %v", err)
	}
	if c.Pub.key == nil {
		t.Fatal("embedded public key not loaded")
	}
}
