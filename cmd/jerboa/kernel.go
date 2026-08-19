package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AitorConS/jerboa/internal/release"
	"github.com/AitorConS/jerboa/internal/tools"
	"github.com/spf13/cobra"
)

// kernelRemoteVersion resolves the latest kernel version from the signed release
// manifest (SHA-256/signature verified). R2 is the single source of truth.
func kernelRemoteVersion(ctx context.Context) (string, error) {
	cl, err := release.Default()
	if err != nil {
		return "", fmt.Errorf("release client: %w", err)
	}
	k, err := tools.KernelComponentFromManifest(ctx, cl, release.ChannelStable)
	if err != nil {
		return "", err
	}
	return k.Version, nil
}

// kernelDownloadLatest installs the latest kernel toolset named by the signed
// manifest, verifying each artifact against its recorded SHA-256.
func kernelDownloadLatest(ctx context.Context, toolsDir string) error {
	cl, err := release.Default()
	if err != nil {
		return fmt.Errorf("release client: %w", err)
	}
	k, err := tools.KernelComponentFromManifest(ctx, cl, release.ChannelStable)
	if err != nil {
		return err
	}
	return tools.DownloadKernelFromManifest(ctx, cl, toolsDir, k)
}

func newKernelCmd(verbose *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kernel",
		Short: "Manage the kernel tools (kernel.img, boot.img, mkfs)",
	}
	cmd.AddCommand(
		newKernelCheckCmd(),
		newKernelUpdateCmd(verbose),
	)
	return cmd
}

// newKernelCheckCmd implements `jerboa kernel check`.
func newKernelCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check whether a newer kernel version is available",
		RunE: func(cmd *cobra.Command, _ []string) error {
			toolsDir := defaultToolsPath()
			local := tools.LocalVersion(toolsDir)
			known := tools.HasLocalVersion(toolsDir)
			fmt.Fprintf(cmd.OutOrStdout(), "Installed kernel: %s\n", local)

			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			remote, err := kernelRemoteVersion(ctx)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Latest kernel:    (unavailable — %v)\n", err)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Latest kernel:    %s\n", remote)

			switch {
			case !known:
				// The toolchain came baked into the distro image and carries no
				// version marker, so its version is genuinely unknown. Claiming
				// "update available" here was a false alarm on every fresh install
				// (F-006): report the state honestly instead.
				fmt.Fprintf(cmd.OutOrStdout(),
					"The installed kernel toolchain shipped with the distro and is unversioned; "+
						"the latest published kernel is %s. Run `jerboa kernel update` to install a CLI-managed copy.\n", remote)
			case tools.IsNewer(local, remote):
				fmt.Fprintf(cmd.OutOrStdout(),
					"Update available. Run `jerboa kernel update` to install %s.\n", remote)
			default:
				fmt.Fprintln(cmd.OutOrStdout(), "Kernel is up to date.")
			}
			return nil
		},
	}
}

// newKernelUpdateCmd implements `jerboa kernel update`.
func newKernelUpdateCmd(verbose *bool) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Download and install the latest kernel version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			toolsDir := defaultToolsPath()
			local := tools.LocalVersion(toolsDir)

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			remote, err := kernelRemoteVersion(ctx)
			if err != nil {
				return fmt.Errorf("kernel update: check remote version: %w", err)
			}

			if !tools.IsNewer(local, remote) {
				fmt.Fprintf(cmd.OutOrStdout(), "Already on the latest kernel (%s).\n", local)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"New kernel version available: %s (installed: %s)\n", remote, local)

			if !yes && !confirmPrompt("Update? [y/N] ") {
				fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
				return nil
			}

			if err := tools.ClearCachedTools(toolsDir); err != nil {
				return fmt.Errorf("kernel update: clear cache: %w", err)
			}

			sp := newSpinner(cmd.ErrOrStderr(), *verbose)
			sp.Start(fmt.Sprintf("Downloading kernel %s", remote))
			dlCtx, dlCancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer dlCancel()
			if err := kernelDownloadLatest(dlCtx, toolsDir); err != nil {
				sp.Fail("Download failed")
				return fmt.Errorf("kernel update: %w", err)
			}
			sp.Done(fmt.Sprintf("Kernel updated to %s", remote))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

// confirmPrompt prints prompt to stderr and reads a y/Y answer from stdin.
// Any other input (including EOF) is treated as "no".
func confirmPrompt(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	ans := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return ans == "y" || ans == "yes"
}
