package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/AitorConS/jerboa/internal/release"
	"github.com/spf13/cobra"
)

// newSelfCmd builds the `jerboa self` group: checking for and applying updates
// to the jerboa CLI itself against the signed release manifest.
func newSelfCmd(verbose *bool) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "self",
		Short: "Inspect and update the jerboa CLI itself",
	}
	cmd.PersistentFlags().StringVar(&channel, "channel", release.ChannelStable,
		"release channel (stable or beta)")
	cmd.AddCommand(
		newSelfCheckCmd(&channel),
		newSelfUpdateCmd(&channel, verbose),
	)
	return cmd
}

// newSelfCheckCmd implements `jerboa self check`.
func newSelfCheckCmd(channel *string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check whether a newer jerboa CLI is available",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Installed jerboa: %s\n", version)

			m, err := fetchManifest(cmd.Context(), *channel)
			if err != nil {
				fmt.Fprintf(out, "Latest jerboa:    (unavailable — %v)\n", err)
				return nil
			}
			cli, ok := m.Component(release.ComponentCLI)
			if !ok {
				return fmt.Errorf("self check: manifest has no cli component")
			}
			fmt.Fprintf(out, "Latest jerboa:    %s (channel %s)\n", cli.Version, m.Channel)
			if release.IsNewer(version, cli.Version) {
				fmt.Fprintf(out, "Update available. Run `jerboa self update` to install %s.\n", cli.Version)
			} else {
				fmt.Fprintln(out, "jerboa is up to date.")
			}
			// Surface the other components for visibility.
			for _, name := range []string{release.ComponentKernel, release.ComponentDaemon, release.ComponentDistro, release.ComponentDesktop} {
				if c, ok := m.Component(name); ok {
					fmt.Fprintf(out, "  %-8s %s\n", name+":", c.Version)
				}
			}
			return nil
		},
	}
}

// newSelfUpdateCmd implements `jerboa self update`.
func newSelfUpdateCmd(channel *string, verbose *bool) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Download and install the latest jerboa CLI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			m, err := fetchManifest(cmd.Context(), *channel)
			if err != nil {
				return fmt.Errorf("self update: %w", err)
			}
			cli, ok := m.Component(release.ComponentCLI)
			if !ok {
				return fmt.Errorf("self update: manifest has no cli component")
			}
			if !release.IsNewer(version, cli.Version) {
				fmt.Fprintf(out, "Already on the latest jerboa (%s).\n", version)
				return nil
			}
			asset, err := cli.Asset(runtime.GOOS, runtime.GOARCH)
			if err != nil {
				return fmt.Errorf("self update: %w", err)
			}

			fmt.Fprintf(out, "New jerboa available: %s (installed: %s)\n", cli.Version, version)
			if !yes && !confirmPrompt("Update? [y/N] ") {
				fmt.Fprintln(out, "Aborted.")
				return nil
			}

			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("self update: locate executable: %w", err)
			}
			exe, err = filepath.EvalSymlinks(exe)
			if err != nil {
				return fmt.Errorf("self update: resolve executable: %w", err)
			}

			cl, err := releaseClient()
			if err != nil {
				return err
			}
			// Stage the new binary next to the current one so the final swap is a
			// rename on the same filesystem (atomic, no cross-device copy).
			staged := exe + ".new"
			sp := newSpinner(cmd.ErrOrStderr(), *verbose)
			sp.Start(fmt.Sprintf("Downloading jerboa %s", cli.Version))
			dlCtx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()
			if err := cl.DownloadArtifact(dlCtx, asset, staged); err != nil {
				sp.Fail("Download failed")
				return fmt.Errorf("self update: %w", err)
			}
			if err := os.Chmod(staged, 0o755); err != nil {
				sp.Fail("Update failed")
				_ = os.Remove(staged)
				return fmt.Errorf("self update: chmod: %w", err)
			}
			if err := replaceExecutable(exe, staged); err != nil {
				sp.Fail("Update failed")
				_ = os.Remove(staged)
				return fmt.Errorf("self update: %w", err)
			}
			sp.Done(fmt.Sprintf("jerboa updated to %s", cli.Version))
			fmt.Fprintln(out, "Restart any running jerboa command to use the new version.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

// replaceExecutable swaps the running binary at exe with the staged file.
//
// A running executable cannot be overwritten in place — Windows locks the file,
// and on Unix an unlink+rename would break a program that re-reads its own path.
// Both platforms do allow *renaming* the running binary, so we move the current
// exe aside to exe.old and rename the staged file into its place. The stale
// exe.old is best-effort cleaned on the next launch (see cleanupStaleSelf).
func replaceExecutable(exe, staged string) error {
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("move current binary aside: %w", err)
	}
	if err := os.Rename(staged, exe); err != nil {
		// Roll back so the user is never left without a working binary.
		_ = os.Rename(old, exe)
		return fmt.Errorf("install new binary: %w", err)
	}
	return nil
}

// cleanupStaleSelf removes a leftover <exe>.old from a previous self update. It
// is best-effort and silent: on Windows the old binary may still be locked
// momentarily after an update, in which case the next run cleans it up.
func cleanupStaleSelf() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return
	}
	_ = os.Remove(exe + ".old")
}

// fetchManifest builds the default release client and fetches the channel
// manifest, verifying its signature.
func fetchManifest(ctx context.Context, channel string) (*release.Manifest, error) {
	cl, err := releaseClient()
	if err != nil {
		return nil, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return cl.FetchManifest(fetchCtx, channel)
}

// releaseClient constructs the shared release client from the embedded public
// key and the default (or JERBOA_RELEASE_BASE-overridden) origin.
func releaseClient() (*release.Client, error) {
	cl, err := release.Default()
	if err != nil {
		return nil, fmt.Errorf("release client: %w", err)
	}
	return cl, nil
}
