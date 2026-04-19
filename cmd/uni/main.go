package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var socketPath string

	root := &cobra.Command{
		Use:     "uni",
		Short:   "Unikernel engine CLI",
		Version: version,
	}
	root.PersistentFlags().StringVar(&socketPath, "socket", "/var/run/unid.sock", "unid daemon socket path")

	root.AddCommand(newRunCmd(&socketPath))
	return root
}
