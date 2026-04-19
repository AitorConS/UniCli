package main

import (
	"fmt"

	"github.com/AitorConS/unikernel-engine/internal/api"
	"github.com/spf13/cobra"
)

func newRunCmd(socketPath *string) *cobra.Command {
	var (
		memory string
		cpus   int
	)
	cmd := &cobra.Command{
		Use:   "run <image>",
		Short: "Create and start a unikernel VM from a disk image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
				ImagePath: args[0],
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
