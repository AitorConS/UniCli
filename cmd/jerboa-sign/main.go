// Command jerboa-sign produces a minisign-compatible signature (.minisig) for a
// file, using a raw Ed25519 seed supplied via the JERBOA_MINISIGN_SEED
// environment variable (base64). It exists so CI can sign the release manifest
// without shipping the minisign binary or storing minisign's encrypted key
// format — the seed is the only secret, and the emitted signature verifies with
// internal/release and with upstream `minisign -V`.
//
// Usage:
//
//	JERBOA_MINISIGN_SEED=<base64 seed> jerboa-sign -in manifest.json [-out manifest.json.minisig] [-key-id <hex>]
//
// The public key's 8-byte key id must be provided (hex) so the signature
// advertises the same id the verifier expects.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	in := flag.String("in", "", "file to sign (required)")
	out := flag.String("out", "", "signature output path (default: <in>.minisig)")
	keyIDHex := flag.String("key-id", "", "8-byte public key id in hex (required)")
	comment := flag.String("comment", "", "trusted comment (default: timestamp)")
	flag.Parse()

	if *in == "" || *keyIDHex == "" {
		fmt.Fprintln(os.Stderr, "jerboa-sign: -in and -key-id are required")
		os.Exit(2)
	}
	dst := *out
	if dst == "" {
		dst = *in + ".minisig"
	}

	seedB64 := os.Getenv("JERBOA_MINISIGN_SEED")
	if seedB64 == "" {
		fmt.Fprintln(os.Stderr, "jerboa-sign: JERBOA_MINISIGN_SEED is not set")
		os.Exit(1)
	}
	seed, err := base64.StdEncoding.DecodeString(seedB64)
	if err != nil || len(seed) != ed25519.SeedSize {
		fmt.Fprintf(os.Stderr, "jerboa-sign: invalid seed (want base64 of %d bytes): %v\n", ed25519.SeedSize, err)
		os.Exit(1)
	}
	keyID, err := hex.DecodeString(*keyIDHex)
	if err != nil || len(keyID) != 8 {
		fmt.Fprintf(os.Stderr, "jerboa-sign: invalid -key-id (want 8-byte hex): %v\n", err)
		os.Exit(1)
	}

	priv := ed25519.NewKeyFromSeed(seed)
	msg, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jerboa-sign: read %s: %v\n", *in, err)
		os.Exit(1)
	}

	tc := *comment
	if tc == "" {
		tc = fmt.Sprintf("timestamp:%d\tfile:%s", time.Now().Unix(), *in)
	}

	// Legacy minisign signature: "Ed" || key id || Ed25519(msg).
	sig := ed25519.Sign(priv, msg)
	blob := append([]byte("Ed"), keyID...)
	blob = append(blob, sig...)
	// Global signature binds sig || trusted comment.
	global := ed25519.Sign(priv, append(append([]byte{}, sig...), []byte(tc)...))

	content := fmt.Sprintf(
		"untrusted comment: signature from jerboa-sign\n%s\ntrusted comment: %s\n%s\n",
		base64.StdEncoding.EncodeToString(blob),
		tc,
		base64.StdEncoding.EncodeToString(global),
	)
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "jerboa-sign: write %s: %v\n", dst, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", dst)
}
