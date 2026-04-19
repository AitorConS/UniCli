package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/AitorConS/unikernel-engine/internal/api"
	"github.com/AitorConS/unikernel-engine/internal/vm"
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
		qemuBin    string
	)
	root := &cobra.Command{
		Use:     "unid",
		Short:   "Unikernel engine daemon",
		Version: version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd.Context(), socketPath, qemuBin)
		},
	}
	root.Flags().StringVar(&socketPath, "socket", "/var/run/unid.sock", "Unix socket path to listen on")
	root.Flags().StringVar(&qemuBin, "qemu", "qemu-system-x86_64", "QEMU binary to use")
	return root
}

func serve(ctx context.Context, socketPath, qemuBin string) error {
	mgr := vm.NewQEMUManager(qemuBin)

	srv, err := api.NewServer(mgr, socketPath)
	if err != nil {
		return fmt.Errorf("unid: %w", err)
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	slog.Info("unid listening", "socket", socketPath, "qemu", qemuBin)
	if err := srv.Serve(ctx); err != nil {
		return fmt.Errorf("unid serve: %w", err)
	}
	slog.Info("unid shutdown complete")
	return nil
}
