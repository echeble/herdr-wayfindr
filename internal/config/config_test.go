package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultHidePrincipalBranchesIsTrue(t *testing.T) {
	cfg := Default()
	if !cfg.Grouping.HidePrincipalBranches {
		t.Error("Default().Grouping.HidePrincipalBranches should be true")
	}
	if len(cfg.Grouping.PrincipalBranches) != 2 || cfg.Grouping.PrincipalBranches[0] != "main" {
		t.Errorf("Default().Grouping.PrincipalBranches = %v, want [main master]", cfg.Grouping.PrincipalBranches)
	}
}

func TestLoadFileHidePrincipalBranches(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "config.toml")

	// 1. Explicitly false
	if err := os.WriteFile(tomlPath, []byte("[grouping]\nhide_principal_branches = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := LoadFile(tomlPath)
	if cfg.Grouping.HidePrincipalBranches {
		t.Error("hide_principal_branches = false should override default")
	}

	// 2. Empty grouping keeps default true
	if err := os.WriteFile(tomlPath, []byte("[grouping]\nuse_worktree_name = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg = LoadFile(tomlPath)
	if !cfg.Grouping.HidePrincipalBranches {
		t.Error("omitted hide_principal_branches should keep default true")
	}
}

func TestWriteResolvedIncludesHidePrincipalBranches(t *testing.T) {
	cfg := Default()
	var buf bytes.Buffer
	if err := WriteResolved(&buf, cfg); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), `"hide_principal_branches": true`) {
		t.Errorf("WriteResolved output does not contain hide_principal_branches: %s", buf.String())
	}
}
