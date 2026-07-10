// Package wsldistro manages the dedicated jerboa WSL2 distribution: a versioned,
// self-contained Linux environment (jerboad + qemu + firecracker + kernel
// toolchain) imported via `wsl --import`, the way Docker Desktop ships its own
// distro. Running the daemon inside it removes any dependency on the user's WSL
// setup, on jerboad being on PATH, or on host sudo.
package wsldistro

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Name is the registered WSL2 distribution name.
const Name = "jerboa"

// RootfsArtifact is the release asset holding the distro root filesystem.
const RootfsArtifact = "jerboa-rootfs-amd64.tar.gz"

// DefaultInstallDir returns where the distro's ext4 disk is stored on Windows
// (%LOCALAPPDATA%\jerboa\distro), falling back to ~/.jerboa/distro.
func DefaultInstallDir() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "jerboa", "distro")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".jerboa", "distro")
	}
	return filepath.Join(home, ".jerboa", "distro")
}

// Exists reports whether the jerboa distro is registered with WSL.
func Exists() (bool, error) {
	names, err := List()
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if strings.EqualFold(n, Name) {
			return true, nil
		}
	}
	return false, nil
}

// List returns the names of all registered WSL2 distributions.
func List() ([]string, error) {
	out, err := exec.Command("wsl", "--list", "--quiet").CombinedOutput() //nolint:gosec,noctx // fixed program, no args
	if err != nil {
		return nil, fmt.Errorf("wsldistro: list distros: %w (%s)", err, strings.TrimSpace(decodeWSLOutput(out)))
	}
	return parseDistroList(out), nil
}

// Import registers the distro from rootfsTar, storing its disk under installDir.
func Import(installDir, rootfsTar string) error {
	if _, err := os.Stat(rootfsTar); err != nil {
		return fmt.Errorf("wsldistro: rootfs %q: %w", rootfsTar, err)
	}
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("wsldistro: create install dir: %w", err)
	}
	out, err := exec.Command("wsl", "--import", Name, installDir, rootfsTar, "--version", "2").CombinedOutput() //nolint:gosec,noctx // controlled args
	if err != nil {
		return fmt.Errorf("wsldistro: import %s: %w (%s)", Name, err, strings.TrimSpace(decodeWSLOutput(out)))
	}
	return nil
}

// IP returns the IPv4 address of the running distro's primary interface. WSL2
// loopback forwarding does not reach a secondary distro, so the Windows client
// dials this address directly. Querying it starts the distro if it is stopped.
func IP() (string, error) {
	out, err := exec.Command("wsl", "-d", Name, "-u", "root", "--", "hostname", "-I").CombinedOutput() //nolint:gosec,noctx // fixed args
	if err != nil {
		return "", fmt.Errorf("wsldistro: distro ip: %w (%s)", err, strings.TrimSpace(decodeWSLOutput(out)))
	}
	fields := strings.Fields(decodeWSLOutput(out))
	if len(fields) == 0 {
		return "", fmt.Errorf("wsldistro: %s has no IP yet", Name)
	}
	return fields[0], nil
}

// DaemonBinaryPath is where jerboad lives inside the distro (baked there by the
// rootfs image and replaced in place on upgrade).
const DaemonBinaryPath = "/usr/local/bin/jerboad"

// InstallDaemonBinary streams the file at localPath into the distro, replacing
// the jerboad binary atomically (write-then-rename, so a crash never leaves a
// truncated binary). Piping over stdin avoids translating the Windows path into
// the distro's /mnt view. The caller should stop the daemon first to avoid
// ETXTBSY on the running binary.
func InstallDaemonBinary(localPath string) error {
	f, err := os.Open(localPath) //nolint:gosec // caller-owned freshly downloaded binary
	if err != nil {
		return fmt.Errorf("wsldistro: open %s: %w", localPath, err)
	}
	defer func() { _ = f.Close() }()

	tmp := DaemonBinaryPath + ".new"
	script := fmt.Sprintf("cat > %[1]s && chmod 0755 %[1]s && mv -f %[1]s %[2]s", tmp, DaemonBinaryPath)
	cmd := exec.Command("wsl", "-d", Name, "-u", "root", "--", "sh", "-c", script) //nolint:gosec,noctx // fixed program, controlled args
	cmd.Stdin = f
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wsldistro: install jerboad: %w (%s)", err, strings.TrimSpace(decodeWSLOutput(out)))
	}
	return nil
}

// DataDirs are the daemon state directories (under $HOME/.jerboa inside the
// distro) that hold user data worth preserving across a rootfs reimport. The
// kernel toolchain cache (tools/) is deliberately excluded — it re-downloads on
// demand, so carrying it would only bloat the archive.
var DataDirs = []string{"images", "vms", "networks", "volumes"}

// ExportData streams a gzip tarball of the distro's persistent data
// (DataDirs under $HOME/.jerboa) to w. Piping over stdout avoids translating a
// Windows path into the distro's /mnt view. When none of the directories exist
// (a fresh distro), it writes nothing and returns nil — the caller detects the
// empty archive by the written size. The daemon should be stopped first so the
// snapshot is consistent.
//
// The script is fed over stdin rather than as a `sh -c` argument on purpose:
// some wsl.exe builds expand every `$NAME` in the command-line arguments
// against the Windows-side environment before invoking the Linux shell, which
// silently blanks the script's own shell variables ($d, $x, $@, $#) and yields
// an empty archive — losing all user data on reimport. Commands read from stdin
// are not subject to that interpolation, so the Linux shell expands them.
func ExportData(w io.Writer) error {
	// Build the tar argument list from only the directories that exist, so
	// tar never fails on a missing path; emit nothing when there is no data.
	script := `d="$HOME/.jerboa"; [ -d "$d" ] || exit 0; cd "$d" || exit 0; ` +
		`set --; for x in ` + strings.Join(DataDirs, " ") + `; do [ -e "$x" ] && set -- "$@" "$x"; done; ` +
		`[ "$#" -eq 0 ] && exit 0; exec tar czf - "$@"`
	cmd := exec.Command("wsl", "-d", Name, "-u", "root", "--", "sh") //nolint:gosec,noctx // fixed program, controlled args
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = w
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wsldistro: export data: %w (%s)", err, strings.TrimSpace(decodeWSLOutput(errb.Bytes())))
	}
	return nil
}

// ImportData restores a gzip tarball previously produced by ExportData into the
// distro, extracting it under ~/.jerboa. Piping over stdin avoids a Windows-path
// translation. Extracting into the freshly imported rootfs auto-starts the
// distro.
//
// The archive occupies stdin, so — unlike ExportData — the script cannot also be
// fed there; it must ride as a `sh -c` argument. To stay safe against wsl.exe
// builds that interpolate `$NAME` in arguments against the Windows-side
// environment (which would blank $HOME and extract into the wrong directory), the
// target is written as an unquoted tilde: `~` carries no `$`, so wsl.exe leaves
// it untouched and the Linux shell expands it to the distro user's home.
func ImportData(r io.Reader) error {
	script := `mkdir -p ~/.jerboa && exec tar xzf - -C ~/.jerboa`
	cmd := exec.Command("wsl", "-d", Name, "-u", "root", "--", "sh", "-c", script) //nolint:gosec,noctx // fixed program, controlled args
	cmd.Stdin = r
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wsldistro: import data: %w (%s)", err, strings.TrimSpace(decodeWSLOutput(out)))
	}
	return nil
}

// Unregister removes the distro and all of its data.
func Unregister() error {
	out, err := exec.Command("wsl", "--unregister", Name).CombinedOutput() //nolint:gosec,noctx // fixed args
	if err != nil {
		return fmt.Errorf("wsldistro: unregister %s: %w (%s)", Name, err, strings.TrimSpace(decodeWSLOutput(out)))
	}
	return nil
}

// parseDistroList extracts distro names from `wsl --list --quiet` output.
func parseDistroList(raw []byte) []string {
	var names []string
	for line := range strings.SplitSeq(decodeWSLOutput(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names
}

// decodeWSLOutput turns wsl.exe's UTF-16LE output into plain text. WSL emits
// UTF-16; dropping the NUL bytes recovers the ASCII distro names and messages.
func decodeWSLOutput(b []byte) string {
	return string(bytes.ReplaceAll(b, []byte{0}, nil))
}
