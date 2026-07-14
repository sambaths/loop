package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultIssueDir = "docs/issues"
const DefaultBranchOrigin = "main"
const DefaultAgentTimeout = 300
const ConfigDirName = ".loop"
const ConfigFileName = "config.json"

type Config struct {
	Repo             string `json:"repo"`
	IssueDir         string `json:"issue_dir"`
	BranchOrigin     string `json:"branch_origin"`
	AgentTimeout     int    `json:"agent_timeout"`
	ChecksumsEnabled bool   `json:"checksums_enabled"`
	BranchFromOrigin bool   `json:"branch_from_origin"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func ConfigExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func DefaultConfig() Config {
	return Config{
		IssueDir:         DefaultIssueDir,
		BranchOrigin:     DefaultBranchOrigin,
		AgentTimeout:     DefaultAgentTimeout,
		ChecksumsEnabled: true,
	}
}

func FindProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	dir := cwd
	for {
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && fi.IsDir() {
			return dir, nil
		}
		if fi, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !fi.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("project root not found (no .git or go.mod from %s)", cwd)
		}
		dir = parent
	}
}

func ConfigPath() (string, error) {
	root, err := FindProjectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ConfigDirName, ConfigFileName), nil
}

func Load() (Config, bool, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, false, nil
		}
		return Config{}, false, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parse config: %w", err)
	}
	return cfg, true, nil
}

func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	ensureGitignore()
	return nil
}

func ensureGitignore() {
	root, err := FindProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not find project root for .gitignore: %v\n", err)
		return
	}
	gitignorePath := filepath.Join(root, ".gitignore")

	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: could not read .gitignore: %v\n", err)
			return
		}
		// File doesn't exist, create it
		if err := os.WriteFile(gitignorePath, []byte(".loop/\n"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not create .gitignore: %v\n", err)
		}
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == ".loop/" || trimmed == ".loop" {
			return // already present
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open .gitignore for append: %v\n", err)
		return
	}
	defer f.Close()

	if _, err := f.WriteString("\n# loop config\n.loop/\n"); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not append to .gitignore: %v\n", err)
	}
}
