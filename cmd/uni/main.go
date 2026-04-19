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
	var (
		socketPath string
		storePath  string
	)

	root := &cobra.Command{
		Use:     "uni",
		Short:   "Unikernel engine CLI",
		Version: version,
	}
	root.PersistentFlags().StringVar(&socketPath, "socket", "/var/run/unid.sock",
		"unid daemon socket path")
	root.PersistentFlags().StringVar(&storePath, "store",
		defaultStorePath(), "local image store path")

	root.AddCommand(
		newRunCmd(&socketPath, &storePath),
		newBuildCmd(&storePath),
		newImagesCmd(&storePath),
		newRmiCmd(&storePath),
		newPushCmd(&storePath),
		newPullCmd(&storePath),
	)
	return root
}

func defaultStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".uni/images"
	}
	return home + "/.uni/images"
}
