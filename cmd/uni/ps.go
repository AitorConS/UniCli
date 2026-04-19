package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/AitorConS/unikernel-engine/internal/api"
	"github.com/spf13/cobra"
)

func newPsCmd(socketPath *string, outputFmt *string) *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List running VMs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := api.Dial(*socketPath)
			if err != nil {
				return fmt.Errorf("ps: connect to daemon: %w", err)
			}
			defer func() {
				if closeErr := client.Close(); closeErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: close client: %v\n", closeErr)
				}
			}()

			infos, err := client.List(cmd.Context())
			if err != nil {
				return fmt.Errorf("ps: %w", err)
			}

			if *outputFmt == "json" {
				return printJSON(cmd.OutOrStdout(), infos)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSTATE\tIMAGE")
			for _, info := range infos {
				fmt.Fprintf(w, "%s\t%s\t%s\n", info.ID, info.State, info.Image)
			}
			return w.Flush()
		},
	}
}
