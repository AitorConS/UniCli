package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	cliGithubAPIBase = "https://api.github.com/repos/AitorConS/UniCLi"
	cliReleaseBase   = "https://github.com/AitorConS/UniCLi/releases/download"
)

func newUpgradeCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade uni (and unid if present) to the latest version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			remote, err := latestCLIVersion(ctx)
			if err != nil {
				return fmt.Errorf("upgrade: check latest version: %w", err)
			}

			local := version // injected at build time via -X main.version
			fmt.Fprintf(cmd.OutOrStdout(), "Installed: %s\n", local)
			fmt.Fprintf(cmd.OutOrStdout(), "Latest:    %s\n", remote)

			if !cliIsNewer(local, remote) {
				fmt.Fprintln(cmd.OutOrStdout(), "Already up to date.")
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "New version available: %s\n", remote)
			if !yes && !confirmPrompt("Upgrade? [y/N] ") {
				fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
				return nil
			}

			dlCtx, dlCancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer dlCancel()

			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("upgrade: locate current binary: %w", err)
			}
			exe, err = filepath.EvalSymlinks(exe)
			if err != nil {
				return fmt.Errorf("upgrade: resolve symlink: %w", err)
			}
			dir := filepath.Dir(exe)

			if err := replaceBinary(dlCtx, cmd, dir, "uni", remote); err != nil {
				return fmt.Errorf("upgrade uni: %w", err)
			}

			// Always upgrade unid alongside uni.
			if err := replaceBinary(dlCtx, cmd, dir, "unid", remote); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: upgrade unid: %v\n", err)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Note: restart unid to apply the new daemon binary.")
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Upgraded to %s.\n", remote)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	cmd.AddCommand(newUpgradeCheckCmd(), newUpgradeListCmd())
	return cmd
}

func newUpgradeCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check whether a newer CLI version is available",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			remote, err := latestCLIVersion(ctx)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Latest: (unavailable — %v)\n", err)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed: %s\n", version)
			fmt.Fprintf(cmd.OutOrStdout(), "Latest:    %s\n", remote)
			if cliIsNewer(version, remote) {
				fmt.Fprintf(cmd.OutOrStdout(),
					"Update available. Run `uni upgrade` to install %s.\n", remote)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Already up to date.")
			}
			return nil
		},
	}
}

func newUpgradeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all available CLI versions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			versions, err := listCLIVersions(ctx)
			if err != nil {
				return fmt.Errorf("upgrade list: %w", err)
			}
			if len(versions) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No releases found.")
				return nil
			}
			for _, v := range versions {
				marker := "  "
				if v == version {
					marker = "* "
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s\n", marker, v)
			}
			return nil
		},
	}
}

// replaceBinary downloads binary `name` at `ver` into `dir` and atomically
// replaces the existing executable.
func replaceBinary(ctx context.Context, cmd *cobra.Command, dir, name, ver string) error {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	artifact := fmt.Sprintf("%s-%s-%s%s", name, runtime.GOOS, runtime.GOARCH, ext)
	url := fmt.Sprintf("%s/%s/%s", cliReleaseBase, ver, artifact)
	dest := filepath.Join(dir, name+ext)

	fmt.Fprintf(cmd.OutOrStdout(), "Downloading %s...\n", artifact)

	tmp, err := os.CreateTemp(dir, name+"-update-*"+ext)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := downloadTo(ctx, url, tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}

	// On Windows rename the running binary away before swapping in the new one.
	if runtime.GOOS == "windows" {
		bak := dest + ".bak"
		_ = os.Remove(bak)
		if err := os.Rename(dest, bak); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("rename current binary: %w", err)
		}
		_ = os.Remove(bak) // best-effort cleanup
	}

	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("install %s: %w", name, err)
	}
	return nil
}

func downloadTo(ctx context.Context, url string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP %d for %s", resp.StatusCode, url)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// latestCLIVersion returns the highest semver CLI release from GitHub.
func latestCLIVersion(ctx context.Context) (string, error) {
	vers, err := listCLIVersions(ctx)
	if err != nil {
		return "", err
	}
	if len(vers) == 0 {
		return "", fmt.Errorf("no CLI releases found")
	}
	return vers[0], nil
}

// listCLIVersions returns all vX.Y.Z CLI release tags, newest-first.
func listCLIVersions(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cliGithubAPIBase+"/releases", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var versions []string
	for _, r := range releases {
		// CLI release tags look like "v0.1.0"; exclude "latest", "kernel-v*".
		tag := r.TagName
		if strings.HasPrefix(tag, "v") && !strings.HasPrefix(tag, "vkernel") {
			versions = append(versions, tag)
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		return cliSemverGT(versions[i], versions[j])
	})
	return versions, nil
}

// cliIsNewer reports whether remote is a strictly higher semver than local.
func cliIsNewer(local, remote string) bool {
	return cliSemverGT(remote, local)
}

func cliSemverGT(a, b string) bool {
	av := cliParseSemver(a)
	bv := cliParseSemver(b)
	for i := range av {
		if av[i] != bv[i] {
			return av[i] > bv[i]
		}
	}
	return false
}

func cliParseSemver(s string) [3]int {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		out[i], _ = strconv.Atoi(p)
	}
	return out
}
