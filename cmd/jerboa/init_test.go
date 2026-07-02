package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AitorConS/jerboa/internal/builder"
)

// runInit executes the init command against dir with extra args.
func runInit(t *testing.T, dir string, extra ...string) (string, error) {
	t.Helper()
	cmd := newInitCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{dir}, extra...))
	err := cmd.Execute()
	return out.String() + errOut.String(), err
}

func TestInitDetectsGo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runInit(t, dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, builder.ConfigFileName))
	if err != nil {
		t.Fatalf("read generated toml: %v", err)
	}
	if !strings.Contains(string(data), `lang = "go"`) {
		t.Fatalf("expected go template, got:\n%s", data)
	}
	// The generated file must parse as a valid config.
	if _, err := builder.LoadConfig(dir); err != nil {
		t.Fatalf("generated unikernel.toml does not validate: %v", err)
	}
}

func TestInitLangOverrideRaw(t *testing.T) {
	dir := t.TempDir()
	if _, err := runInit(t, dir, "--lang", "raw"); err != nil {
		t.Fatalf("init: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, builder.ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `lang = "raw"`) || !strings.Contains(content, "[program]") {
		t.Fatalf("expected raw template with [program] section, got:\n%s", content)
	}
	if _, err := builder.LoadConfig(dir); err != nil {
		t.Fatalf("generated unikernel.toml does not validate: %v", err)
	}
}

func TestInitRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, builder.ConfigFileName)
	if err := os.WriteFile(target, []byte("# existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runInit(t, dir, "--lang", "go"); err == nil {
		t.Fatal("expected error when unikernel.toml exists")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "# existing\n" {
		t.Fatal("existing file was modified without --force")
	}
	if _, err := runInit(t, dir, "--lang", "go", "--force"); err != nil {
		t.Fatalf("init --force: %v", err)
	}
}

func TestInitTemplatesValidate(t *testing.T) {
	// Every language template must produce a config that parses and validates.
	for _, l := range []builder.Lang{builder.LangGo, builder.LangNode, builder.LangPython, builder.LangRust, builder.LangRaw, builder.LangUnknown} {
		dir := t.TempDir()
		content := initTemplate(l)
		if err := os.WriteFile(filepath.Join(dir, builder.ConfigFileName), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := builder.LoadConfig(dir); err != nil {
			t.Errorf("template for %s does not validate: %v", templateLangName(l), err)
		}
	}
}
