package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AitorConS/jerboa/internal/builder"
	"github.com/spf13/cobra"
)

// newInitCmd scaffolds a commented unikernel.toml in a project directory, so
// the first build starts from a file that already explains the knobs and the
// unikernel-specific pitfalls instead of a blank page.
func newInitCmd() *cobra.Command {
	var (
		lang  string
		force bool
	)
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Create a commented unikernel.toml for a project",
		Long: `Create a unikernel.toml in the given directory (default: current directory).

The build language is auto-detected from project markers (go.mod, package.json,
pyproject.toml/requirements.txt, Cargo.toml); use --lang to override, including
--lang raw for prebuilt runtimes (databases, JVM apps, ...) that come from
packages instead of compilation.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			dir = absPath(dir)
			info, err := os.Stat(dir)
			if err != nil {
				return fmt.Errorf("init: stat %s: %w", dir, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("init: %s is not a directory", dir)
			}

			target := filepath.Join(dir, builder.ConfigFileName)
			if _, err := os.Stat(target); err == nil && !force {
				return fmt.Errorf("init: %s already exists (use --force to overwrite)", target)
			}

			detected := builder.LangUnknown
			if lang != "" {
				detected, err = builder.ParseLang(lang)
				if err != nil {
					return fmt.Errorf("init: %w", err)
				}
			} else {
				// Best effort: an undetectable or ambiguous project falls back to
				// the raw template, which documents the package-driven path.
				if d, derr := builder.DetectLanguage(dir, builder.LangUnknown); derr == nil {
					detected = d
				}
			}

			content := initTemplate(detected)
			if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
				return fmt.Errorf("init: write %s: %w", target, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s (lang = %q)\n", target, templateLangName(detected))
			fmt.Fprintf(cmd.ErrOrStderr(), "Next: jerboa build %s --name <image-name>\n", dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&lang, "lang", "", "build language for the template (go, node, python, rust, raw); default: auto-detect")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing unikernel.toml")
	return cmd
}

func templateLangName(l builder.Lang) string {
	if l == builder.LangUnknown {
		return "raw"
	}
	return l.String()
}

// initTemplate returns a fully commented unikernel.toml for the language.
// The comments deliberately explain unikernel concepts inline — single-process
// kernel, packages, program path resolution — because the file is most users'
// first contact with them.
func initTemplate(l builder.Lang) string {
	header := `# unikernel.toml — build + run spec for a Jerboa unikernel image.
#
# One file describes how the image is assembled (like a Dockerfile) and the
# defaults it runs with (like a compose service). Build with:
#
#   jerboa build . --name <image-name>
#
# A unikernel runs exactly ONE program in its own VM: there is no shell, no
# init system, and no fork/exec — anything that spawns child processes will
# not work inside the guest. See docs/build-concepts.md for the full model.

`
	runSection := `
[run]
# Defaults baked into the image and inherited by 'jerboa run' unless a flag
# overrides them (flag > [run] value > built-in default: 256M / 1 CPU).
memory = "256M"
cpus = 1
# Default port publishes (host:guest), applied when the VM joins a network
# ('jerboa run --network <name>') and no -p flag is given. Publishing requires
# a managed network: create one with 'jerboa network create <name>'.
# ports = ["8080:8080"]
`
	switch l {
	case builder.LangGo:
		return header + `[build]
lang = "go"

# Optional: package path of the main package when it is not the module root.
# entrypoint = "./cmd/server"

# Extra 'go build' arguments. The build always uses CGO_ENABLED=0 and targets
# linux/amd64, producing the static ELF binary a unikernel needs.
# args = ["-tags", "netgo"]

# Shell commands to run before compiling (the Dockerfile RUN equivalent).
# run = ["go generate ./..."]

# Reserve free space in the image for runtime writes (logs, temp files).
# Without it the image is sized to its contents and writes fail with ENOSPC.
# disk_size = "512M"

# Absolute directories to create empty inside the image — needed as volume
# mount points and for paths the program writes to.
# dirs = ["/data"]
` + runSection
	case builder.LangNode:
		return header + `[build]
lang = "node"

# Entrypoint script. Default: the "main" field of package.json, then index.js.
# entrypoint = "server.js"

# The Node runtime is added automatically as a package (version taken from
# "engines.node" in package.json, default 20). npm dependencies are installed
# with 'npm ci --omit=dev' when node_modules/ is absent.

# Shell commands to run before packaging (e.g. transpile/bundle steps).
# run = ["npm run build"]
` + runSection
	case builder.LangPython:
		return header + `[build]
lang = "python"

# Entrypoint script. Default: [project.scripts] from pyproject.toml, then main.py.
# entrypoint = "app.py"

# The Python runtime is added automatically as a package (version taken from
# "requires-python" in pyproject.toml, default 3.12). requirements.txt is
# installed with pip into packages/ as Linux x86_64 wheels — packages that only
# ship source distributions (no manylinux wheel) cannot be used.

# Shell commands to run before packaging.
# run = ["python manage.py collectstatic"]
` + runSection
	case builder.LangRust:
		return header + `[build]
lang = "rust"

# Rust builds cross-compile with: cargo build --release --target x86_64-unknown-linux-musl
# The musl target produces the static ELF binary a unikernel needs. Install it once:
#   rustup target add x86_64-unknown-linux-musl

# Extra cargo arguments.
# args = ["--features", "prod"]
` + runSection
	default: // raw and unknown
		return header + `[build]
# "raw" performs no compilation: the program comes prebuilt from a package.
# Use it for databases, JVM apps, and any runtime without a dedicated driver.
lang = "raw"

# Packages to include. With pkg_source = "ops" these come from the nanovms/ops
# ecosystem (search with 'jerboa pkg search <term>'). Declaring them here makes
# 'jerboa build .' self-contained — no --pkg flags needed.
# pkgs = ["eyberg/postgresql:11.3.0"]
pkg_source = "ops"

# Reserve free space in the image for runtime writes (database files, logs).
# disk_size = "1G"

# Absolute directories to create empty inside the image (volume mount points,
# scratch paths the program writes to).
# dirs = ["/data"]

[program]
# The program to run, resolved against the package files. Give the FULL
# in-image path (e.g. "/usr/local/pgsql/bin/postgres"), not a bare name: some
# packages ship a same-named launcher stub at the image root, and programs that
# locate their install prefix via /proc/self/exe only work from their real path.
#
# If this whole [program] section is omitted, the build falls back to the
# Program/Args declared by the ops package's own package.manifest.
# path = "/usr/local/bin/example"

# argv[1..] passed to the program (argv[0] is the program path itself).
# args = ["--flag", "value"]

[env]
# Environment variables baked into the image (highest priority, overriding
# package and driver env for the same key).
# KEY = "value"
` + runSection
	}
}
