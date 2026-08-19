package main

import (
	"testing"

	"github.com/AitorConS/jerboa/internal/builder"
	"github.com/stretchr/testify/require"
)

// TestCopyFromGuestPath is the F-032 regression: a multi-stage copy_from must
// place the artifact at its declared dst, not silently at the source binary's
// basename. The previous code computed dst then discarded it (`_ = dst`).
func TestCopyFromGuestPath(t *testing.T) {
	cases := []struct {
		name       string
		cf         builder.CopyFromConfig
		prevBinary string
		want       string
	}{
		{
			name:       "explicit dst is honored",
			cf:         builder.CopyFromConfig{Stage: "builder", Src: "/app/server", Dst: "usr/bin/server"},
			prevBinary: "/tmp/build123/server",
			want:       "usr/bin/server",
		},
		{
			name:       "leading slash trimmed",
			cf:         builder.CopyFromConfig{Stage: "builder", Src: "/app/server", Dst: "/opt/app/server"},
			prevBinary: "/tmp/build123/server",
			want:       "opt/app/server",
		},
		{
			name:       "no dst falls back to src basename",
			cf:         builder.CopyFromConfig{Stage: "builder", Src: "/app/myserver"},
			prevBinary: "/tmp/build123/out",
			want:       "myserver",
		},
		{
			name:       "no dst and no src falls back to produced binary basename",
			cf:         builder.CopyFromConfig{Stage: "builder"},
			prevBinary: "/tmp/build123/finalbin",
			want:       "finalbin",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, copyFromGuestPath(tc.cf, tc.prevBinary))
		})
	}
}
