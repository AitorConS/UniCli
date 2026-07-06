package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/AitorConS/jerboa/internal/release"
	"github.com/AitorConS/jerboa/internal/tools"
	"github.com/AitorConS/jerboa/internal/wslboot"
	"github.com/AitorConS/jerboa/internal/wsldistro"
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

// newSelfUpdateCmd implements `jerboa self update`. By default it updates only
// the CLI; --kernel, --daemon, and --all extend it to the rest of the toolset.
func newSelfUpdateCmd(channel *string, verbose *bool) *cobra.Command {
	var yes, doKernel, doDaemon, doAll bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Download and install the latest jerboa toolset",
		Long: "Update the jerboa CLI against the signed release manifest.\n\n" +
			"By default only the CLI is updated. Add --kernel to also refresh the\n" +
			"kernel toolset, --daemon to replace jerboad inside the WSL2 distro\n" +
			"(preserving its data), or --all for the CLI, kernel, and daemon together.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := fetchManifest(cmd.Context(), *channel)
			if err != nil {
				return fmt.Errorf("self update: %w", err)
			}

			wantKernel := doKernel || doAll
			wantDaemon := doDaemon || doAll

			// Compatibility gate: never install a daemon the CLI that will drive
			// it is too old to speak to. The effective CLI is the just-updated one
			// when we are also updating the CLI, else the currently running build.
			if wantDaemon {
				effectiveCLI := version
				if cli, ok := m.Component(release.ComponentCLI); ok && release.IsNewer(version, cli.Version) {
					effectiveCLI = cli.Version
				}
				if err := m.CheckDaemonCompat(effectiveCLI); err != nil {
					return fmt.Errorf("self update: %w", err)
				}
			}

			if err := updateSelfCLI(cmd, m, yes, verbose); err != nil {
				return err
			}
			if wantKernel {
				if err := updateKernel(cmd, verbose); err != nil {
					return err
				}
			}
			if wantDaemon {
				if err := updateDaemon(cmd, m, yes, verbose); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompts")
	cmd.Flags().BoolVar(&doKernel, "kernel", false, "also update the kernel toolset")
	cmd.Flags().BoolVar(&doDaemon, "daemon", false, "also update the daemon (jerboad) in the WSL2 distro")
	cmd.Flags().BoolVar(&doAll, "all", false, "update the CLI, kernel, and daemon together")
	return cmd
}

// updateSelfCLI swaps the running jerboa binary for the manifest's CLI build.
func updateSelfCLI(cmd *cobra.Command, m *release.Manifest, yes bool, verbose *bool) error {
	out := cmd.OutOrStdout()
	cli, ok := m.Component(release.ComponentCLI)
	if !ok {
		return fmt.Errorf("self update: manifest has no cli component")
	}
	if !release.IsNewer(version, cli.Version) {
		fmt.Fprintf(out, "CLI already on the latest jerboa (%s).\n", version)
		return nil
	}
	asset, err := cli.Asset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("self update: %w", err)
	}

	fmt.Fprintf(out, "New jerboa available: %s (installed: %s)\n", cli.Version, version)
	if !yes && !confirmPrompt("Update the CLI? [y/N] ") {
		fmt.Fprintln(out, "Skipped CLI update.")
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
}

// updateKernel installs the latest kernel toolset (manifest-first) when newer.
func updateKernel(cmd *cobra.Command, verbose *bool) error {
	out := cmd.OutOrStdout()
	toolsDir := defaultToolsPath()
	local := tools.LocalVersion(toolsDir)

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	remote, err := kernelRemoteVersion(ctx)
	if err != nil {
		return fmt.Errorf("self update kernel: %w", err)
	}
	if !tools.IsNewer(local, remote) {
		fmt.Fprintf(out, "Kernel already up to date (%s).\n", local)
		return nil
	}
	if err := tools.ClearCachedTools(toolsDir); err != nil {
		return fmt.Errorf("self update kernel: clear cache: %w", err)
	}
	sp := newSpinner(cmd.ErrOrStderr(), *verbose)
	sp.Start(fmt.Sprintf("Downloading kernel %s", remote))
	dlCtx, dlCancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
	defer dlCancel()
	if err := kernelDownloadLatest(dlCtx, toolsDir); err != nil {
		sp.Fail("Download failed")
		return fmt.Errorf("self update kernel: %w", err)
	}
	sp.Done(fmt.Sprintf("Kernel updated to %s", remote))
	return nil
}

// updateDaemon replaces jerboad inside the WSL2 distro with the manifest build,
// preserving the distro's data. It is Windows-only because the daemon runs in
// the dedicated distro; the swap stops the daemon, streams the new binary in,
// and restarts it.
func updateDaemon(cmd *cobra.Command, m *release.Manifest, yes bool, verbose *bool) error {
	out := cmd.OutOrStdout()
	if runtime.GOOS != "windows" {
		return errNotWindows("self update --daemon")
	}
	if err := requireDistro(); err != nil {
		return err
	}
	d, ok := m.Component(release.ComponentDaemon)
	if !ok {
		return fmt.Errorf("self update: manifest has no daemon component")
	}
	// jerboad runs inside the Linux distro regardless of the host architecture.
	asset, err := d.Asset("linux", "amd64")
	if err != nil {
		return fmt.Errorf("self update daemon: %w", err)
	}

	fmt.Fprintf(out, "Updating daemon to %s ...\n", d.Version)
	if !yes && !confirmPrompt("Stop the running daemon and replace jerboad? [y/N] ") {
		fmt.Fprintln(out, "Skipped daemon update.")
		return nil
	}

	cl, err := releaseClient()
	if err != nil {
		return err
	}
	// Stage on the host, then stream into the distro. Remove the placeholder so
	// DownloadArtifact's rename(dest.tmp -> dest) does not fail on Windows.
	tmp, err := os.CreateTemp("", "jerboad-*")
	if err != nil {
		return fmt.Errorf("self update daemon: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("self update daemon: close temp: %w", err)
	}
	if err := os.Remove(tmpPath); err != nil {
		return fmt.Errorf("self update daemon: remove temp placeholder: %w", err)
	}
	defer func() { _ = os.Remove(tmpPath) }()

	sp := newSpinner(cmd.ErrOrStderr(), *verbose)
	sp.Start(fmt.Sprintf("Downloading jerboad %s", d.Version))
	dlCtx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
	defer cancel()
	if err := cl.DownloadArtifact(dlCtx, asset, tmpPath); err != nil {
		sp.Fail("Download failed")
		return fmt.Errorf("self update daemon: %w", err)
	}
	sp.Done(fmt.Sprintf("Downloaded jerboad %s", d.Version))

	wcfg, token, err := resolveDaemonConfig(daemonOpts{})
	if err != nil {
		return err
	}
	if err := wslboot.Stop(wcfg.Distro, wcfg.User); err != nil && !errors.Is(err, wslboot.ErrNoDaemon) {
		return fmt.Errorf("self update daemon: stop: %w", err)
	}
	waitPortReleased(cmd.Context(), wcfg.Endpoint, token, 5*time.Second)
	if err := wsldistro.InstallDaemonBinary(tmpPath); err != nil {
		return fmt.Errorf("self update daemon: %w", err)
	}
	fmt.Fprintln(out, "daemon binary updated; restarting ...")
	return launchAndWait(cmd, wcfg, token)
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
