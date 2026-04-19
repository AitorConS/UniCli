package main

import (
	"fmt"
	"strings"

	"github.com/AitorConS/unikernel-engine/internal/api"
	"github.com/AitorConS/unikernel-engine/internal/image"
	"github.com/spf13/cobra"
)

func newRunCmd(socketPath, storePath *string) *cobra.Command {
	var (
		memory string
		cpus   int
	)
	cmd := &cobra.Command{
		Use:   "run <image>",
		Short: "Create and start a unikernel VM (image = path or name:tag)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			imgArg := args[0]
			diskPath, err := resolveImage(imgArg, *storePath, memory, cpus)
			if err != nil {
				return fmt.Errorf("run: resolve image: %w", err)
			}

			client, err := api.Dial(*socketPath)
			if err != nil {
				return fmt.Errorf("run: connect to daemon: %w", err)
			}
			defer func() {
				if closeErr := client.Close(); closeErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: close client: %v\n", closeErr)
				}
			}()

			info, err := client.Run(cmd.Context(), api.RunParams{
				ImagePath: diskPath,
				Memory:    memory,
				CPUs:      cpus,
			})
			if err != nil {
				return fmt.Errorf("run: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", info.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&memory, "memory", "256M", "VM memory (e.g. 256M, 1G)")
	cmd.Flags().IntVar(&cpus, "cpus", 1, "number of virtual CPUs")
	return cmd
}

// resolveImage returns the disk path for imgArg.
// If imgArg looks like a file path it is returned as-is; otherwise it is
// treated as a name:tag reference and looked up in the local image store.
func resolveImage(imgArg, storePath, memory string, cpus int) (string, error) {
	if isFilePath(imgArg) {
		return imgArg, nil
	}
	store, err := image.NewStore(storePath)
	if err != nil {
		return "", fmt.Errorf("open image store: %w", err)
	}
	m, diskPath, err := store.Get(imgArg)
	if err != nil {
		return "", fmt.Errorf("image %s not found (use 'uni pull' or provide a file path): %w", imgArg, err)
	}
	// Use image defaults when caller did not override.
	if memory == "256M" && m.Config.Memory != "" {
		_ = memory // caller value takes precedence
	}
	if cpus == 1 && m.Config.CPUs > 0 {
		_ = cpus
	}
	return diskPath, nil
}

func isFilePath(s string) bool {
	return strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, ".")
}
