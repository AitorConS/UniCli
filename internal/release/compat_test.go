package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckDaemonCompat(t *testing.T) {
	withDaemon := func(minCLI string) *Manifest {
		return &Manifest{
			Channel: "stable",
			Components: map[string]Component{
				ComponentDaemon: {Version: "v1.0.0", MinCLI: minCLI},
			},
		}
	}

	t.Run("cli newer than min is compatible", func(t *testing.T) {
		require.NoError(t, withDaemon("v0.4.0").CheckDaemonCompat("v0.5.0"))
	})
	t.Run("cli equal to min is compatible", func(t *testing.T) {
		require.NoError(t, withDaemon("v0.5.0").CheckDaemonCompat("v0.5.0"))
	})
	t.Run("cli older than min is rejected", func(t *testing.T) {
		err := withDaemon("v0.5.0").CheckDaemonCompat("v0.4.0")
		require.Error(t, err)
		var ce *CompatError
		require.ErrorAs(t, err, &ce)
		assert.Equal(t, "v0.4.0", ce.CLIVersion)
		assert.Equal(t, "v0.5.0", ce.MinCLI)
	})
	t.Run("no min_cli imposes no constraint", func(t *testing.T) {
		require.NoError(t, withDaemon("").CheckDaemonCompat("v0.0.1"))
	})
	t.Run("no daemon component imposes no constraint", func(t *testing.T) {
		m := &Manifest{Channel: "stable", Components: map[string]Component{
			ComponentCLI: {Version: "v1", URL: "https://x", SHA256: "ab"},
		}}
		require.NoError(t, m.CheckDaemonCompat("v0.0.1"))
	})
	t.Run("unknown cli version fails closed", func(t *testing.T) {
		require.Error(t, withDaemon("v0.1.0").CheckDaemonCompat("(unknown)"))
	})
}
