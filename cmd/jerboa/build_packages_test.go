package main

import "testing"

func TestSameRuntimeFamily(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"python", "python", true},
		{"python", "python3", true},
		{"python3", "python", true},
		{"node", "node20", true},
		{"node", "node-exporter", false}, // shared prefix, not a version suffix
		{"python3", "python2", false},    // different versions
		{"node", "deno", false},
		{"go", "golang", false},
	}
	for _, tt := range tests {
		if got := sameRuntimeFamily(tt.a, tt.b); got != tt.want {
			t.Errorf("sameRuntimeFamily(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestFilterCoveredAutoPkgsKeepsUnrelated(t *testing.T) {
	auto := []string{"node"}
	user := []string{"node-exporter"}
	got := filterCoveredAutoPkgs(auto, user)
	if len(got) != 1 || got[0] != "node" {
		t.Fatalf("expected node runtime kept, got %v", got)
	}
}

func TestMergePkgRefs(t *testing.T) {
	tests := []struct {
		name       string
		fromConfig []string
		fromFlags  []string
		want       []string
	}{
		{"config only", []string{"eyberg/mysql:5.7.29"}, nil, []string{"eyberg/mysql:5.7.29"}},
		{"flags only", nil, []string{"node:20"}, []string{"node:20"}},
		{"config first then flags", []string{"a"}, []string{"b"}, []string{"a", "b"}},
		{"exact duplicates dropped", []string{"a", "b"}, []string{"b", "a", "c"}, []string{"a", "b", "c"}},
		{"empty refs dropped", []string{""}, []string{"a", ""}, []string{"a"}},
		{"both empty", nil, nil, nil},
	}
	for _, tt := range tests {
		got := mergePkgRefs(tt.fromConfig, tt.fromFlags)
		if len(got) != len(tt.want) {
			t.Errorf("%s: mergePkgRefs = %v, want %v", tt.name, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("%s: mergePkgRefs = %v, want %v", tt.name, got, tt.want)
				break
			}
		}
	}
}
