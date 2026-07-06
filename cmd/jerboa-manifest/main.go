// Command jerboa-manifest generates a release channel manifest from a spec.
//
// The spec names each component's version and local artifact paths; the tool
// hashes those files, derives their published URLs from the bucket convention
// (<base>/<component>/<version>/<basename>), and writes a manifest.json ready to
// be signed with jerboa-sign and uploaded to R2.
//
// Usage:
//
//	jerboa-manifest -spec spec.json -out channels/stable.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/AitorConS/jerboa/internal/release"
)

func main() {
	specPath := flag.String("spec", "", "path to the manifest spec JSON (required)")
	outPath := flag.String("out", "", "output manifest path (required)")
	flag.Parse()

	if *specPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "jerboa-manifest: -spec and -out are required")
		os.Exit(2)
	}

	raw, err := os.ReadFile(*specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jerboa-manifest: read spec: %v\n", err)
		os.Exit(1)
	}
	var spec release.Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		fmt.Fprintf(os.Stderr, "jerboa-manifest: parse spec: %v\n", err)
		os.Exit(1)
	}

	m, err := release.BuildManifest(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jerboa-manifest: %v\n", err)
		os.Exit(1)
	}
	out, err := m.Marshal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jerboa-manifest: marshal: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outPath, append(out, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "jerboa-manifest: write %s: %v\n", *outPath, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d components)\n", *outPath, len(m.Components))
}
