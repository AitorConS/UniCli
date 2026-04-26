package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/AitorConS/unikernel-engine/internal/image"
	"github.com/AitorConS/unikernel-engine/internal/tools"
	"github.com/spf13/cobra"
)

// absPath resolves p to an absolute path, returning p unchanged on error.
func absPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func newBuildCmd(storePath *string) *cobra.Command {
	var (
		name      string
		tag       string
		memory    string
		cpus      int
		mkfs      string
		updateYes bool
	)
	cmd := &cobra.Command{
		Use:   "build <binary>",
		Short: "Build a unikernel image from a static ELF binary",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := image.NewStore(*storePath)
			if err != nil {
				return fmt.Errorf("build: open store: %w", err)
			}

			toolsDir := defaultToolsPath()

			// Check for a newer kernel version when tools are already cached.
			// Skip if the user provided a custom --mkfs path.
			if mkfs == "" && os.Getenv("UNI_MKFS") == "" && tools.Exist(toolsDir) {
				if err := checkKernelUpdateForBuild(cmd, toolsDir, updateYes); err != nil {
					return err
				}
			}

			// Resolve mkfs: use --mkfs override if given, otherwise auto-download.
			if mkfs == "" {
				mkfs = os.Getenv("UNI_MKFS")
			}
			mkfsRun, err := tools.ResolveMkfs(cmd.Context(), toolsDir, mkfs)
			if err != nil {
				return fmt.Errorf("build: %w", err)
			}

			binaryPath := absPath(args[0])
			if name == "" {
				name = args[0]
			}
			m, err := image.NewBuilder(store).Build(cmd.Context(), image.BuildConfig{
				Name:       name,
				Tag:        tag,
				BinaryPath: binaryPath,
				MkfsRun:    mkfsRun,
				Memory:     memory,
				CPUs:       cpus,
			})
			if err != nil {
				return fmt.Errorf("build: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %s:%s\n", m.DiskDigest, m.Name, m.Tag)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "image name (default: binary filename)")
	cmd.Flags().StringVar(&tag, "tag", "latest", "image tag")
	cmd.Flags().StringVar(&memory, "memory", "256M", "default VM memory")
	cmd.Flags().IntVar(&cpus, "cpus", 1, "default VM CPU count")
	cmd.Flags().StringVar(&mkfs, "mkfs", "", "path to mkfs binary — skip auto-download (env: UNI_MKFS)")
	cmd.Flags().BoolVarP(&updateYes, "update-kernel", "U", false, "auto-approve kernel update if one is available")
	return cmd
}

// checkKernelUpdateForBuild fetches the remote kernel version and, if it differs
// from the local one, asks the user whether to update before building.
func checkKernelUpdateForBuild(cmd *cobra.Command, toolsDir string, autoYes bool) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 8*time.Second)
	defer cancel()

	remote, err := tools.RemoteVersion(ctx)
	if err != nil {
		// Network unreachable: silently continue, don't block the build.
		return nil
	}
	local := tools.LocalVersion(toolsDir)
	if local == remote {
		return nil
	}

	fmt.Fprintf(cmd.ErrOrStderr(),
		"⚠  New kernel version available: %s (installed: %s)\n", remote, local)

	if !autoYes && !confirmPrompt("Update kernel before building? [y/N] ") {
		return nil
	}

	if err := tools.ClearCachedTools(toolsDir); err != nil {
		return fmt.Errorf("build: clear kernel cache: %w", err)
	}
	dlCtx, dlCancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
	defer dlCancel()
	if _, err := tools.ResolveMkfs(dlCtx, toolsDir, ""); err != nil {
		return fmt.Errorf("build: download new kernel: %w", err)
	}
	if err := tools.SaveLocalVersion(toolsDir, remote); err != nil {
		return fmt.Errorf("build: save kernel version: %w", err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Kernel updated to %s.\n", remote)
	return nil
}

func defaultToolsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".uni", "tools")
	}
	return filepath.Join(home, ".uni", "tools")
}
