package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AitorConS/unikernel-engine/internal/api"
	"github.com/AitorConS/unikernel-engine/internal/tools"
	"github.com/spf13/cobra"
)

func newCpCmd(socketPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "cp <src> <dst>",
		Short: "Copy files to or from a stopped VM",
		Long: `Copy files to or from a stopped VM disk image.

Currently only copying FROM a stopped VM is supported, and it requires the
'dump' tool built from kernel/tools/dump.c to extract files from the TFS
filesystem used by Nanos. The tool is downloaded automatically on first use.

Example:
  uni cp myvm:/etc/config.json ./config.json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, dst := args[0], args[1]
			fromVM, srcID, srcVMPath := parseCpSpec(src)
			toVM, dstID, dstVMPath := parseCpSpec(dst)

			if fromVM && toVM {
				return fmt.Errorf("cp: cannot copy between two VMs")
			}
			if !fromVM && !toVM {
				return fmt.Errorf("cp: at least one operand must be a VM reference (id:path)")
			}

			client, err := api.Dial(*socketPath)
			if err != nil {
				return fmt.Errorf("cp: connect to daemon: %w", err)
			}
			defer func() {
				if closeErr := client.Close(); closeErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: close client: %v\n", closeErr)
				}
			}()

			vmID := ""
			vmPath := ""
			localPath := ""
			if fromVM {
				vmID = srcID
				vmPath = srcVMPath
				localPath = dst
			}
			if toVM {
				vmID = dstID
				vmPath = dstVMPath
				localPath = src
			}

			detail, err := client.Inspect(cmd.Context(), vmID)
			if err != nil {
				return fmt.Errorf("cp: %w", err)
			}
			if detail.State != "stopped" {
				return fmt.Errorf("cp: VM %s is %s; cp only works on stopped VMs", vmID, detail.State)
			}

			if toVM {
				return fmt.Errorf("cp: copying to a VM image is not yet supported")
			}

			// fromVM: extract file from stopped VM disk image.
			dumpBin, err := tools.ResolveDump(cmd.Context(), defaultToolsPath(), "")
			if err != nil {
				return fmt.Errorf("cp: resolve dump tool: %w", err)
			}

			tmpDir, err := os.MkdirTemp("", "uni-cp-*")
			if err != nil {
				return fmt.Errorf("cp: create temp dir: %w", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			// dump extracts the entire filesystem to tmpDir.
			execCmd := exec.Command(dumpBin, "-f", detail.Image, "-o", tmpDir)
			if out, err := execCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("cp: dump failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
			}

			srcFile := filepath.Join(tmpDir, filepath.FromSlash(vmPath))
			if _, err := os.Stat(srcFile); err != nil {
				return fmt.Errorf("cp: file not found in VM image: %s", vmPath)
			}

			if err := copyFile(srcFile, localPath); err != nil {
				return fmt.Errorf("cp: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "copied %s:%s → %s\n", vmID, vmPath, localPath)
			return nil
		},
	}
}

// parseCpSpec parses a string like "id:/path" into (isVM, id, path).
// For non-VM paths it returns (false, "", s).
func parseCpSpec(s string) (bool, string, string) {
	if idx := strings.Index(s, ":"); idx > 0 {
		return true, s[:idx], s[idx+1:]
	}
	return false, "", s
}

// copyFile copies a regular file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}
	return nil
}
