package pkg

import "testing"

func TestParseDockerConfig(t *testing.T) {
	data := []byte(`{"Entrypoint":["/usr/local/bin/redis-server"],"Cmd":["--appendonly","yes"],"Env":["PATH=/usr/local/bin","REDIS_VERSION=7.2"]}`)
	cfg, err := ParseDockerConfig(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.ProgramCandidate(); got != "/usr/local/bin/redis-server" {
		t.Fatalf("program candidate = %q", got)
	}
	args := cfg.ProgramArgs()
	if len(args) != 2 || args[0] != "--appendonly" || args[1] != "yes" {
		t.Fatalf("program args = %v", args)
	}
	if len(cfg.Env) != 2 {
		t.Fatalf("env = %v", cfg.Env)
	}
}

func TestParseDockerConfigCmdOnly(t *testing.T) {
	cfg, err := ParseDockerConfig([]byte(`{"Cmd":["node","server.js"]}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.ProgramCandidate(); got != "node" {
		t.Fatalf("program candidate = %q", got)
	}
	args := cfg.ProgramArgs()
	if len(args) != 1 || args[0] != "server.js" {
		t.Fatalf("program args = %v", args)
	}
}

func TestParseDockerConfigEmpty(t *testing.T) {
	cfg, err := ParseDockerConfig([]byte(`{}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := cfg.ProgramCandidate(); got != "" {
		t.Fatalf("expected empty candidate, got %q", got)
	}
	if args := cfg.ProgramArgs(); args != nil {
		t.Fatalf("expected nil args, got %v", args)
	}
}

func TestParseDockerConfigInvalid(t *testing.T) {
	if _, err := ParseDockerConfig([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestIsShellLauncher(t *testing.T) {
	shell := []string{
		"/usr/local/bin/docker-entrypoint.sh",
		"docker-entrypoint.sh",
		"sh", "/bin/sh", "bash", "/usr/bin/tini", "dumb-init",
	}
	for _, s := range shell {
		if !IsShellLauncher(s) {
			t.Errorf("IsShellLauncher(%q) = false, want true", s)
		}
	}
	notShell := []string{
		"/usr/local/bin/redis-server",
		"node",
		"/usr/local/pgsql/bin/postgres",
		"java",
	}
	for _, s := range notShell {
		if IsShellLauncher(s) {
			t.Errorf("IsShellLauncher(%q) = true, want false", s)
		}
	}
}
