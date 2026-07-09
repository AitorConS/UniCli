package main

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/AitorConS/jerboa/internal/release"
	"github.com/AitorConS/jerboa/internal/tools"
	"github.com/spf13/cobra"
)

// newVersionCmd builds `jerboa version`: a read-only readout of the installed
// CLI and kernel versions alongside the latest published versions of every
// component (CLI, kernel, daemon, distro, desktop) from the signed manifest.
// It never installs anything — updates are delivered by the desktop app on
// Windows and by the OS package/reinstall on Linux.
func newVersionCmd() *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show installed versions and the latest available release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "jerboa CLI:  %s\n", version)
			fmt.Fprintf(out, "kernel:      %s\n", tools.LocalVersion(defaultToolsPath()))

			m, err := fetchChannelManifest(cmd.Context(), channel)
			if err != nil {
				fmt.Fprintf(out, "\nLatest release: (unavailable — %v)\n", err)
				return nil
			}
			fmt.Fprintf(out, "\nLatest release (channel %s):\n", m.Channel)
			for _, name := range []string{
				release.ComponentCLI, release.ComponentKernel, release.ComponentDaemon,
				release.ComponentDistro, release.ComponentDesktop,
			} {
				if c, ok := m.Component(name); ok {
					fmt.Fprintf(out, "  %-8s %s\n", name+":", c.Version)
				}
			}
			if cli, ok := m.Component(release.ComponentCLI); ok && release.IsNewer(version, cli.Version) {
				fmt.Fprintf(out, "\nA newer jerboa CLI (%s) is available.\n", cli.Version)
				if runtime.GOOS == "windows" {
					fmt.Fprintln(out, "Update through the Jerboa Desktop app.")
				} else {
					fmt.Fprintln(out, "Reinstall jerboa to update (see https://releases.jerboa.dev).")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", release.ChannelStable,
		"release channel (stable or beta)")
	return cmd
}

// fetchChannelManifest fetches and signature-verifies the channel manifest.
func fetchChannelManifest(ctx context.Context, channel string) (*release.Manifest, error) {
	cl, err := release.Default()
	if err != nil {
		return nil, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return cl.FetchManifest(fetchCtx, channel)
}
