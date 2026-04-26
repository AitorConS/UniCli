package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AitorConS/unikernel-engine/internal/tools"
	"github.com/spf13/cobra"
)

func newKernelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kernel",
		Short: "Manage the kernel tools (kernel.img, boot.img, mkfs)",
	}
	cmd.AddCommand(newKernelCheckCmd(), newKernelUpdateCmd())
	return cmd
}

// newKernelCheckCmd implements `uni kernel check`.
func newKernelCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check whether a newer kernel version is available",
		RunE: func(cmd *cobra.Command, _ []string) error {
			toolsDir := defaultToolsPath()
			local := tools.LocalVersion(toolsDir)
			fmt.Fprintf(cmd.OutOrStdout(), "Local kernel version:  %s\n", local)

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			remote, err := tools.RemoteVersion(ctx)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Remote kernel version: (unavailable — %v)\n", err)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Remote kernel version: %s\n", remote)

			if local == remote {
				fmt.Fprintln(cmd.OutOrStdout(), "Kernel is up to date.")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(),
					"Update available. Run `uni kernel update` to install %s.\n", remote)
			}
			return nil
		},
	}
}

// newKernelUpdateCmd implements `uni kernel update`.
func newKernelUpdateCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Download and install the latest kernel tools",
		RunE: func(cmd *cobra.Command, _ []string) error {
			toolsDir := defaultToolsPath()
			local := tools.LocalVersion(toolsDir)

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			remote, err := tools.RemoteVersion(ctx)
			if err != nil {
				return fmt.Errorf("kernel update: check remote version: %w", err)
			}

			if local == remote {
				fmt.Fprintf(cmd.OutOrStdout(), "Already on the latest kernel (%s).\n", local)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"New kernel version available: %s (installed: %s)\n", remote, local)

			if !yes && !confirmPrompt("Update kernel tools? [y/N] ") {
				fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
				return nil
			}

			if err := tools.ClearCachedTools(toolsDir); err != nil {
				return fmt.Errorf("kernel update: clear cache: %w", err)
			}

			dlCtx, dlCancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer dlCancel()
			if _, err := tools.ResolveMkfs(dlCtx, toolsDir, ""); err != nil {
				return fmt.Errorf("kernel update: download: %w", err)
			}
			if err := tools.SaveLocalVersion(toolsDir, remote); err != nil {
				return fmt.Errorf("kernel update: save version: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Kernel updated to %s.\n", remote)
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
