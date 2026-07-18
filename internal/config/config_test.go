package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func resolveDir(dir string) string {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return resolved
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.IssueDir != DefaultIssueDir {
		t.Errorf("expected IssueDir %q, got %q", DefaultIssueDir, cfg.IssueDir)
	}
	if cfg.BranchOrigin != DefaultBranchOrigin {
		t.Errorf("expected BranchOrigin %q, got %q", DefaultBranchOrigin, cfg.BranchOrigin)
	}
	if cfg.Repo != "" {
		t.Errorf("expected empty Repo, got %q", cfg.Repo)
	}
	if cfg.AgentTimeout != DefaultAgentTimeout {
		t.Errorf("expected AgentTimeout %d, got %d", DefaultAgentTimeout, cfg.AgentTimeout)
	}
	if !cfg.ChecksumsEnabled {
		t.Error("expected ChecksumsEnabled to default to true")
	}
	if cfg.InactivityWarn != DefaultInactivityWarn {
		t.Errorf("expected InactivityWarn %d, got %d", DefaultInactivityWarn, cfg.InactivityWarn)
	}
	if cfg.InactivityRecover != DefaultInactivityRecover {
		t.Errorf("expected InactivityRecover %d, got %d", DefaultInactivityRecover, cfg.InactivityRecover)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := Config{
		Repo:             "my-org/my-repo",
		IssueDir:         "custom/issues",
		BranchOrigin:     "develop",
		AgentTimeout:     600,
		ChecksumsEnabled: false,
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, exists, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !exists {
		t.Fatal("expected config to exist")
	}
	if loaded.Repo != cfg.Repo {
		t.Errorf("expected Repo %q, got %q", cfg.Repo, loaded.Repo)
	}
	if loaded.IssueDir != cfg.IssueDir {
		t.Errorf("expected IssueDir %q, got %q", cfg.IssueDir, loaded.IssueDir)
	}
	if loaded.BranchOrigin != cfg.BranchOrigin {
		t.Errorf("expected BranchOrigin %q, got %q", cfg.BranchOrigin, loaded.BranchOrigin)
	}
	if loaded.AgentTimeout != cfg.AgentTimeout {
		t.Errorf("expected AgentTimeout %d, got %d", cfg.AgentTimeout, loaded.AgentTimeout)
	}
	if loaded.ChecksumsEnabled != cfg.ChecksumsEnabled {
		t.Errorf("expected ChecksumsEnabled %v, got %v", cfg.ChecksumsEnabled, loaded.ChecksumsEnabled)
	}
}

func TestLoadMissingConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	_, exists, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if exists {
		t.Fatal("expected config to not exist")
	}
}

func TestConfigPath(t *testing.T) {
	dir := resolveDir(t.TempDir())
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath failed: %v", err)
	}
	expected := filepath.Join(dir, ConfigDirName, ConfigFileName)
	if path != expected {
		t.Errorf("expected path %q, got %q", expected, path)
	}
}

func TestFindProjectRoot_FindsGit(t *testing.T) {
	dir := resolveDir(t.TempDir())
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	root, err := FindProjectRoot()
	if err != nil {
		t.Fatalf("FindProjectRoot failed: %v", err)
	}
	if root != dir {
		t.Errorf("expected root %q, got %q", dir, root)
	}
}

func TestFindProjectRoot_FindsGoMod(t *testing.T) {
	dir := resolveDir(t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	root, err := FindProjectRoot()
	if err != nil {
		t.Fatalf("FindProjectRoot failed: %v", err)
	}
	if root != dir {
		t.Errorf("expected root %q, got %q", dir, root)
	}
}

func TestFindProjectRoot_WalksUp(t *testing.T) {
	dir := resolveDir(t.TempDir())
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	subdir := filepath.Join(dir, "a", "b", "c")
	os.MkdirAll(subdir, 0755)
	origWd, _ := os.Getwd()
	os.Chdir(subdir)
	defer os.Chdir(origWd)

	root, err := FindProjectRoot()
	if err != nil {
		t.Fatalf("FindProjectRoot failed: %v", err)
	}
	if root != dir {
		t.Errorf("expected root %q, got %q", dir, root)
	}
}

func TestFindProjectRoot_NotFound(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	_, err := FindProjectRoot()
	if err == nil {
		t.Fatal("expected error when no .git or go.mod exists")
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := Config{
		Repo:         "my-org/my-repo",
		IssueDir:     "custom/issues",
		BranchOrigin: "develop",
		AgentTimeout: 600,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.Repo != cfg.Repo {
		t.Errorf("expected Repo %q, got %q", cfg.Repo, loaded.Repo)
	}
	if loaded.IssueDir != cfg.IssueDir {
		t.Errorf("expected IssueDir %q, got %q", cfg.IssueDir, loaded.IssueDir)
	}
	if loaded.BranchOrigin != cfg.BranchOrigin {
		t.Errorf("expected BranchOrigin %q, got %q", cfg.BranchOrigin, loaded.BranchOrigin)
	}
	if loaded.AgentTimeout != cfg.AgentTimeout {
		t.Errorf("expected AgentTimeout %d, got %d", cfg.AgentTimeout, loaded.AgentTimeout)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.json")

	cfg := Config{
		Repo:         "my-org/my-repo",
		IssueDir:     "custom/issues",
		BranchOrigin: "develop",
		AgentTimeout: 600,
	}

	if err := SaveConfig(path, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.Repo != cfg.Repo {
		t.Errorf("expected Repo %q, got %q", cfg.Repo, loaded.Repo)
	}
	if loaded.IssueDir != cfg.IssueDir {
		t.Errorf("expected IssueDir %q, got %q", cfg.IssueDir, loaded.IssueDir)
	}
	if loaded.BranchOrigin != cfg.BranchOrigin {
		t.Errorf("expected BranchOrigin %q, got %q", cfg.BranchOrigin, loaded.BranchOrigin)
	}
	if loaded.AgentTimeout != cfg.AgentTimeout {
		t.Errorf("expected AgentTimeout %d, got %d", cfg.AgentTimeout, loaded.AgentTimeout)
	}
}

func TestConfigExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if ConfigExists(path) {
		t.Fatal("expected ConfigExists to return false for missing file")
	}

	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	if !ConfigExists(path) {
		t.Fatal("expected ConfigExists to return true for existing file")
	}
}

func TestSaveCreatesDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := DefaultConfig()
	cfg.Repo = "test/repo"

	if err := Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	configDir := filepath.Join(dir, ConfigDirName)
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Fatal("expected config directory to exist")
	}
}

func TestConfigPath_NoProjectRoot(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	_, err := ConfigPath()
	if err == nil {
		t.Fatal("expected error when no project root")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ConfigDirName), 0755)
	if err := os.WriteFile(filepath.Join(dir, ConfigDirName, ConfigFileName), []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	_, _, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_NoProjectRoot(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	_, _, err := Load()
	if err == nil {
		t.Fatal("expected error when no project root")
	}
}

func TestSave_NoProjectRoot(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	err := Save(DefaultConfig())
	if err == nil {
		t.Fatal("expected error when no project root")
	}
}

func TestLoadConfig_EmptyObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Repo != "" {
		t.Errorf("expected empty Repo, got %q", cfg.Repo)
	}
	if cfg.IssueDir != "" {
		t.Errorf("expected empty IssueDir, got %q", cfg.IssueDir)
	}
	if cfg.BranchOrigin != "" {
		t.Errorf("expected empty BranchOrigin, got %q", cfg.BranchOrigin)
	}
}

func TestLoadConfigWithAgentTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"agent_timeout": 600}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.AgentTimeout != 600 {
		t.Errorf("expected AgentTimeout 600, got %d", cfg.AgentTimeout)
	}
}

func TestFindProjectRoot_GitFileNotDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../.git/worktree\n"), 0644); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	_, err := FindProjectRoot()
	if err == nil {
		t.Fatal("expected error when .git is a file, not a directory")
	}
}
