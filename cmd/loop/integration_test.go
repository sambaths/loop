//go:build integration

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sambaths/loop/internal/config"
	"github.com/sambaths/loop/internal/runner"
)

func buildAndPath(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "loop")
	rootBytes, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root := strings.TrimSpace(string(rootBytes))
	build := exec.Command("go", "build", "-o", bin, "./cmd/loop")
	build.Dir = root
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

func TestIntegrationHelpFlag(t *testing.T) {
	bin := buildAndPath(t)
	out, err := exec.Command(bin, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help failed: %v", err)
	}
	if !strings.Contains(string(out), "Usage:") {
		t.Error("expected 'Usage:' in help output")
	}
	if !strings.Contains(string(out), "loop [command]") {
		t.Error("expected 'loop [command]' in help output")
	}
}

func TestIntegrationHelpShort(t *testing.T) {
	bin := buildAndPath(t)
	out, err := exec.Command(bin, "-h").CombinedOutput()
	if err != nil {
		t.Fatalf("-h failed: %v", err)
	}
	if !strings.Contains(string(out), "Usage:") {
		t.Error("expected 'Usage:' in -h output")
	}
}

func TestIntegrationHelpSubcommand(t *testing.T) {
	bin := buildAndPath(t)
	out, err := exec.Command(bin, "help").CombinedOutput()
	if err != nil {
		t.Fatalf("help subcommand failed: %v", err)
	}
	if !strings.Contains(string(out), "Usage:") {
		t.Error("expected 'Usage:' in help output")
	}
}

func TestIntegrationVersion(t *testing.T) {
	bin := buildAndPath(t)
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "loop v") {
		t.Errorf("expected 'loop v' in version output, got %q", got)
	}
	if !strings.Contains(got, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Errorf("expected OS/arch in version output, got %q", got)
	}
}

func TestIntegrationUnknownCommand(t *testing.T) {
	bin := buildAndPath(t)
	cmd := exec.Command(bin, "unknown")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected exit error for unknown command")
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 2 {
			t.Errorf("expected exit code 2, got %d", exitErr.ExitCode())
		}
	} else {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if !strings.Contains(string(out), "Usage:") {
		t.Error("expected 'Usage:' in error output")
	}
}

func TestIntegrationStatusWithConfig(t *testing.T) {
	dir := t.TempDir()
	mkGitDir(t, dir)
	writeConfig(t, dir, `{"issue_dir":"docs/issues"}`)
	mkIssuesDir(t, filepath.Join(dir, "docs", "issues"))

	bin := buildAndPath(t)
	cmd := exec.Command(bin, "status")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "no issues found") {
		t.Errorf("expected 'no issues found', got %q", string(out))
	}
}

func TestIntegrationStatusWithIssues(t *testing.T) {
	dir := t.TempDir()
	mkGitDir(t, dir)
	writeConfig(t, dir, `{"issue_dir":"docs/issues","repo":"test/repo"}`)

	issuesDir := filepath.Join(dir, "docs", "issues")
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "001-first.md"), "# Issue A\n\nBody A")
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "002-second.md"), "# Issue B\n\nBody B")
	writeIssue(t, filepath.Join(issuesDir, "done", "003-done.md"), "# Issue C\n\nBody C")
	writeIssue(t, filepath.Join(issuesDir, ".quarantine", "004-bad.md"), "# Issue D\n\nBody D")

	bin := buildAndPath(t)
	cmd := exec.Command(bin, "status")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "test-ready:  2") {
		t.Errorf("expected test-ready count 2, got %q", string(out))
	}
	if !strings.Contains(string(out), "done:        1") {
		t.Errorf("expected done count 1, got %q", string(out))
	}
	if !strings.Contains(string(out), "quarantined: 1") {
		t.Errorf("expected quarantined count 1, got %q", string(out))
	}
	if !strings.Contains(string(out), "test/repo") {
		t.Errorf("expected repo info, got %q", string(out))
	}
}

func TestIntegrationRunWithConfig(t *testing.T) {
	dir := t.TempDir()
	mkGitDir(t, dir)
	writeConfig(t, dir, `{"issue_dir":"docs/issues"}`)

	bin := buildAndPath(t)
	cmd := exec.Command(bin, "run", "5")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "no issues found in pipeline") {
		t.Errorf("expected 'no issues found in pipeline', got %q", string(out))
	}
}

func TestIntegrationRunCtrlC(t *testing.T) {
	dir := t.TempDir()

	// .git dir for FindProjectRoot.
	mkGitDir(t, dir)

	issuesDir := filepath.Join(dir, "docs", "issues")
	mkIssuesDir(t, issuesDir)

	writeIssue(t, filepath.Join(issuesDir, "001-test.md"),
		"# Test Issue\n\nExecution mode: AFK-only\n")

	cfg := config.Config{IssueDir: issuesDir}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	// Fake opencode that blocks so we can verify the agent process is killed
	// when the context is cancelled.
	fakeBin := filepath.Join(dir, "fakebin")
	os.MkdirAll(fakeBin, 0755)
	if err := os.WriteFile(filepath.Join(fakeBin, "opencode"), []byte("#!/bin/sh\nsleep 60\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(fakeBin, "opencode"), 0755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", fakeBin+":"+origPath)
	defer os.Setenv("PATH", origPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var logBuf bytes.Buffer
	errCh := make(chan error, 1)

	go func() {
		err := runner.RunLoopStreamed(ctx, &cfg, 1, 0, false,
			func(line string) { logBuf.WriteString(line + "\n") },
			func(iter, total int, title, role string) {},
		)
		errCh <- err
	}()

	// Wait for agent to start (the fake opencode sleep is running).
	time.Sleep(1 * time.Second)

	// Simulate Ctrl+C — cancel the context.
	cancel()

	// Verify the run loop returns promptly.
	select {
	case runErr := <-errCh:
		if runErr == nil {
			t.Error("expected non-nil error after context cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunLoopStreamed did not return within 5s of cancellation")
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "iteration") {
		t.Errorf("expected iteration log output, got:\n%s", logs)
	}
}

// helpers

func mkGitDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
}

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	configDir := filepath.Join(dir, ".loop")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func mkIssuesDir(t *testing.T, dir string) {
	t.Helper()
	for _, sub := range []string{"test-ready", "done", ".quarantine"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func writeIssue(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
