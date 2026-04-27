package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/AitorConS/unikernel-engine/internal/api"
	"github.com/AitorConS/unikernel-engine/internal/image"
	"github.com/AitorConS/unikernel-engine/internal/vm"
	"github.com/AitorConS/unikernel-engine/internal/volume"
	"github.com/spf13/cobra"
)

func newRunCmd(socketPath, storePath *string) *cobra.Command {
	var (
		memory  string
		cpus    int
		ports   []string
		envs    []string
		envFile string
		name    string
		rm      bool
		volumes []string
		attach  bool
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

			portMaps, err := vm.ParsePortMaps(ports)
			if err != nil {
				return fmt.Errorf("run: %w", err)
			}

			env, err := buildEnv(envs, envFile)
			if err != nil {
				return fmt.Errorf("run: %w", err)
			}

			volSpecs, err := resolveVolumes(volumes, *storePath)
			if err != nil {
				return fmt.Errorf("run: %w", err)
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

			params := api.RunParams{
				ImagePath:  diskPath,
				Memory:     memory,
				CPUs:       cpus,
				Env:        env,
				Name:       name,
				AutoRemove: rm,
				Volumes:    volSpecs,
				Attach:     attach,
			}
			for _, pm := range portMaps {
				params.PortMaps = append(params.PortMaps, api.PortMapSpec{
					HostPort:  pm.HostPort,
					GuestPort: pm.GuestPort,
					Protocol:  string(pm.Protocol),
				})
			}

			info, err := client.Run(cmd.Context(), params)
			if err != nil {
				return fmt.Errorf("run: %w", err)
			}

			if attach {
				if err := client.Attach(cmd.Context(), info.ID, cmd.OutOrStdout()); err != nil {
					return fmt.Errorf("run: attach: %w", err)
				}
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", info.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&memory, "memory", "256M", "VM memory (e.g. 256M, 1G)")
	cmd.Flags().IntVar(&cpus, "cpus", 1, "number of virtual CPUs")
	cmd.Flags().StringArrayVarP(&ports, "port", "p", nil, "publish port(s): host:guest[/tcp|udp] (repeatable)")
	cmd.Flags().StringArrayVarP(&envs, "env", "e", nil, "set environment variable KEY=VALUE (repeatable)")
	cmd.Flags().StringVar(&envFile, "env-file", "", "read environment variables from file (one KEY=VALUE per line)")
	cmd.Flags().StringVar(&name, "name", "", "assign a name to the VM instance")
	cmd.Flags().BoolVar(&rm, "rm", false, "automatically remove the VM when it stops")
	cmd.Flags().StringArrayVarP(&volumes, "volume", "v", nil, "mount a volume: name:guestpath[:ro] (repeatable)")
	cmd.Flags().BoolVar(&attach, "attach", false, "attach to VM serial console (blocks until VM stops)")
	return cmd
}

// resolveImage returns the disk path for imgArg.
// If imgArg looks like a file path it is returned as-is; otherwise it is
// treated as a name:tag reference and looked up in the local image store.
func resolveImage(imgArg, storePath, memory string, cpus int) (string, error) {
	if isFilePath(imgArg) {
		if err := rejectELF(imgArg); err != nil {
			return "", err
		}
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

// rejectELF returns an error if path is an ELF binary instead of a disk image.
func rejectELF(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return nil // let QEMU produce the real error
	}
	defer func() { _ = f.Close() }()
	magic := make([]byte, 4)
	if _, err := f.Read(magic); err != nil {
		return nil
	}
	if magic[0] == 0x7f && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F' {
		return fmt.Errorf("%s is an ELF binary, not a bootable disk image.\nRun 'uni build --name <name> %s' first, then 'uni run <name>:latest'", path, path)
	}
	return nil
}

func isFilePath(s string) bool {
	if strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, ".") {
		return true
	}
	// Windows absolute paths: C:\ or C:/
	return len(s) >= 3 && s[1] == ':' && (s[2] == '/' || s[2] == '\\')
}

// buildEnv merges -e flags with an optional --env-file.
// File lines starting with # or empty are ignored.
func buildEnv(envFlags []string, envFile string) ([]string, error) {
	result := make([]string, 0, len(envFlags))
	result = append(result, envFlags...)

	if envFile == "" {
		return result, nil
	}
	f, err := os.Open(envFile)
	if err != nil {
		return nil, fmt.Errorf("open env-file %s: %w", envFile, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		result = append(result, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env-file %s: %w", envFile, err)
	}
	return result, nil
}

// resolveVolumes converts "-v name:guestpath[:ro]" specs to VolumeMountSpec.
// The volume name is resolved to a disk path via the volume store.
func resolveVolumes(specs []string, storePath string) ([]api.VolumeMountSpec, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	volRoot := volumeStorePath(storePath)
	store, err := volume.NewStore(volRoot)
	if err != nil {
		return nil, fmt.Errorf("open volume store: %w", err)
	}

	out := make([]api.VolumeMountSpec, 0, len(specs))
	for _, spec := range specs {
		mount, err := parseVolumeSpec(spec, store)
		if err != nil {
			return nil, err
		}
		out = append(out, mount)
	}
	return out, nil
}

// parseVolumeSpec parses "name:guestpath" or "name:guestpath:ro".
func parseVolumeSpec(spec string, store *volume.Store) (api.VolumeMountSpec, error) {
	parts := strings.Split(spec, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return api.VolumeMountSpec{}, fmt.Errorf("volume spec %q: expected name:guestpath[:ro]", spec)
	}
	name := parts[0]
	guestPath := parts[1]
	readOnly := len(parts) == 3 && strings.EqualFold(parts[2], "ro")

	vol, err := store.Get(name)
	if err != nil {
		return api.VolumeMountSpec{}, fmt.Errorf("volume %q not found (create with 'uni volume create %s'): %w", name, name, err)
	}
	return api.VolumeMountSpec{
		DiskPath:  vol.DiskPath,
		GuestPath: guestPath,
		ReadOnly:  readOnly,
	}, nil
}

// parseVolumePortString parses a port spec string reusing vm.ParsePortMap.
// Exported to share with compose.go within the same package.
func parseVolumePortString(s string) (vm.PortMap, error) {
	return vm.ParsePortMap(s)
}

func volumeStorePath(storePath string) string {
	// Volumes live alongside images but in their own subdirectory.
	idx := strings.LastIndexAny(storePath, "/\\")
	if idx < 0 {
		return "volumes"
	}
	return storePath[:idx+1] + "volumes"
}
