package main

import (
	"fmt"
	"os"

	"github.com/AitorConS/unikernel-engine/internal/image"
	"github.com/spf13/cobra"
)

func newBuildCmd(storePath *string) *cobra.Command {
	var (
		name   string
		tag    string
		memory string
		cpus   int
		mkfs   string
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
			if mkfs == "" {
				mkfs = os.Getenv("UNI_MKFS")
			}
			if mkfs == "" {
				mkfs = "kernel/output/tools/bin/mkfs"
			}
			if name == "" {
				name = args[0]
			}
			m, err := image.NewBuilder(store).Build(cmd.Context(), image.BuildConfig{
				Name:       name,
				Tag:        tag,
				BinaryPath: args[0],
				MkfsBin:    mkfs,
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
	cmd.Flags().StringVar(&mkfs, "mkfs", "", "path to mkfs binary (env: UNI_MKFS)")
	return cmd
}
