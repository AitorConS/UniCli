package release

import "fmt"

// CompatError reports that a CLI is too old to drive the daemon a manifest
// ships. It carries both versions so callers can print an actionable message.
type CompatError struct {
	CLIVersion string // the CLI that would be talking to the daemon
	MinCLI     string // daemon.min_cli: the oldest CLI the daemon speaks to
}

func (e *CompatError) Error() string {
	return fmt.Sprintf(
		"jerboa %s is too old for this release's daemon (needs %s or newer); update jerboa first",
		e.CLIVersion, e.MinCLI)
}

// CheckDaemonCompat verifies the CLI at cliVersion is new enough to drive the
// daemon this manifest ships. The daemon declares the oldest CLI it speaks to in
// its min_cli field; a CLI older than that must not apply or talk to the daemon.
//
// A manifest with no daemon component, or a daemon with no min_cli, imposes no
// constraint and returns nil. A malformed cliVersion sorts as 0.0.0, so it is
// treated as older than any real min_cli (fails closed).
func (m *Manifest) CheckDaemonCompat(cliVersion string) error {
	d, ok := m.Component(ComponentDaemon)
	if !ok || d.MinCLI == "" {
		return nil
	}
	// IsNewer(cliVersion, min) is true exactly when cliVersion < min.
	if IsNewer(cliVersion, d.MinCLI) {
		return &CompatError{CLIVersion: cliVersion, MinCLI: d.MinCLI}
	}
	return nil
}
