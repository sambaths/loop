package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/sambaths/loop/internal/agent"
	"github.com/sambaths/loop/internal/config"
	"github.com/sambaths/loop/internal/issue"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	for _, kv := range []string{"user.name test", "user.email test@test.com"} {
		parts := strings.Split(kv, " ")
		cmd := exec.Command("git", "config", parts[0], parts[1])
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git config %s failed: %v\n%s", kv, err, out)
		}
	}
	cmd = exec.Command("git", "commit", "--allow-empty", "-m", "initial")
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

func setupMockOpencode(t *testing.T, result string, exitCode int) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get caller info")
	}
	mockSrc := filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "mock-opencode.sh")

	data, err := os.ReadFile(mockSrc)
	if err != nil {
		t.Fatalf("read mock script: %v", err)
	}

	mockDir := t.TempDir()
	mockPath := filepath.Join(mockDir, "opencode")
	if err := os.WriteFile(mockPath, data, 0755); err != nil {
		t.Fatalf("write mock binary: %v", err)
	}

	t.Setenv("MOCK_OPENDCODE_RESULT", result)
	t.Setenv("MOCK_OPENDCODE_EXIT_CODE", strconv.Itoa(exitCode))

	return mockDir
}

func createIssueFile(t *testing.T, dir string, state issue.State, title string) *issue.IssueFile {
	t.Helper()
	fullBody := "# " + title + "\n\nExecution mode: AFK-only\n\n## What to build\n\nTest content\n"
	iss, err := issue.Create(dir, state, title, fullBody)
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	return &issue.IssueFile{
		Title:    iss.Title,
		FilePath: iss.FilePath,
		State:    iss.State,
	}
}

// setupMockOpencodeConditional creates a mock opencode that outputs a promise
// on the second call only (for recovery testing). The first call outputs no promise.
// For calls where stdin contains the recovery prompt, it outputs the recoveryResult.
func setupMockOpencodeConditional(t *testing.T, recoveryResult string) string {
	t.Helper()
	mockDir := t.TempDir()
	mockPath := filepath.Join(mockDir, "opencode")
	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail
input=$(cat)
echo "$input"
# If stdin contains the recovery prompt, output a promise
if echo "$input" | grep -qF "missing a promise marker"; then
    echo "__LOOP_RESULT__"
    echo "%s"
    echo "__LOOP_RESULT_END__"
fi
exit 0
`, recoveryResult)
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock binary: %v", err)
	}
	return mockDir
}

func TestRunIterationComplete(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)
	issueFile := createIssueFile(t, dir, issue.StateTodo, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.Complete {
		t.Errorf("expected COMPLETE, got %q", promise)
	}
}

func TestRunIterationTestPass(t *testing.T) {
	dir := t.TempDir()
	issueFile := createIssueFile(t, dir, issue.StateTestReady, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencode(t, "TEST_PASS", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleTest)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.TestPass {
		t.Errorf("expected TEST_PASS, got %q", promise)
	}
}

func TestRunIterationTestFail(t *testing.T) {
	dir := t.TempDir()
	issueFile := createIssueFile(t, dir, issue.StateTestReady, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencode(t, "TEST_FAIL", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleTest)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.TestFail {
		t.Errorf("expected TEST_FAIL, got %q", promise)
	}
}

func TestRunIterationNoMoreTasks(t *testing.T) {
	dir := t.TempDir()
	issueFile := createIssueFile(t, dir, issue.StateTodo, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencode(t, "NO_MORE_TASKS", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.NoMoreTasks {
		t.Errorf("expected NO_MORE_TASKS, got %q", promise)
	}
}

func TestRunIterationInvalidPromiseCausesRecovery(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)
	issueFile := createIssueFile(t, dir, issue.StateTodo, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	// Mock always outputs INVALID (not a valid promise), so recovery also fails
	mockDir := setupMockOpencode(t, "INVALID", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err != nil {
		t.Fatalf("RunIteration should not return error for invalid promise, got: %v", err)
	}
	if promise != agent.TestFail {
		t.Errorf("expected TEST_FAIL fallback for invalid promise, got %q", promise)
	}
}

func TestRunIterationNonZeroExitFallsBackToTestFail(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)
	issueFile := createIssueFile(t, dir, issue.StateTodo, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencode(t, "INVALID", 1)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err != nil {
		t.Fatalf("RunIteration should not return error for non-zero exit with invalid promise, got: %v", err)
	}
	if promise != agent.TestFail {
		t.Errorf("expected TEST_FAIL fallback for non-zero exit, got %q", promise)
	}
}

func TestRunIterationMissingIssueFile(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	issueFile := &issue.IssueFile{
		Title:    "Missing",
		FilePath: filepath.Join(dir, "nonexistent.md"),
		State:    issue.StateTodo,
	}

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	_, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err == nil {
		t.Fatal("expected error for missing issue file")
	}
}

func TestRunIterationCustomBranchOrigin(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	issueFile := createIssueFile(t, dir, issue.StateTodo, "Test Issue")

	exec.Command("git", "add", filepath.Base(issueFile.FilePath)).Run()
	exec.Command("git", "commit", "-m", "add issue").Run()

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60, BranchOrigin: "main"}

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.Complete {
		t.Errorf("expected COMPLETE, got %q", promise)
	}
}

func TestRunIterationEmptyBranchOrigin(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)
	issueFile := createIssueFile(t, dir, issue.StateTodo, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60, BranchOrigin: ""}

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.Complete {
		t.Errorf("expected COMPLETE, got %q", promise)
	}
}

func TestRunIterationPromiseRecovery(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)
	issueFile := createIssueFile(t, dir, issue.StateTodo, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencodeConditional(t, "COMPLETE")
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.Complete {
		t.Errorf("expected COMPLETE from recovery, got %q", promise)
	}
}

func TestRunIterationPromiseRecoveryFails(t *testing.T) {
	dir := t.TempDir()
	issueFile := createIssueFile(t, dir, issue.StateTestReady, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencodeConditional(t, "")
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleTest)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.TestFail {
		t.Errorf("expected TEST_FAIL default when recovery also fails, got %q", promise)
	}
}

func TestRunIterationZeroTimeout(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)
	issueFile := createIssueFile(t, dir, issue.StateTodo, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 0}

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.Complete {
		t.Errorf("expected COMPLETE, got %q", promise)
	}
}

func TestRunIterationImplementStripsTestResults(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)
	body := "# Test Issue\n\nExecution mode: AFK-only\n\n## What to build\n\nTest content\n\n## Test Results\n\nAll tests passed\n\n## UAT Results\n\nAll good\n"
	iss, err := issue.Create(dir, issue.StateTodo, "Test Issue", body)
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	issueFile := &issue.IssueFile{
		Title:    iss.Title,
		FilePath: iss.FilePath,
		State:    iss.State,
	}
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.Complete {
		t.Errorf("expected COMPLETE, got %q", promise)
	}
}

func TestRunIterationTestKeepsTestResults(t *testing.T) {
	dir := t.TempDir()
	body := "# Test Issue\n\nExecution mode: AFK-only\n\n## What to build\n\nTest content\n\n## Test Results\n\nAll tests passed\n\n## UAT Results\n\nAll good\n"
	iss, err := issue.Create(dir, issue.StateTestReady, "Test Issue", body)
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	issueFile := &issue.IssueFile{
		Title:    iss.Title,
		FilePath: iss.FilePath,
		State:    iss.State,
	}
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencode(t, "TEST_PASS", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleTest)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.TestPass {
		t.Errorf("expected TEST_PASS, got %q", promise)
	}
}

func TestRunIterationImplementStripsWithNoResults(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)
	issueFile := createIssueFile(t, dir, issue.StateTodo, "Clean Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	promise, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err != nil {
		t.Fatalf("RunIteration failed: %v", err)
	}
	if promise != agent.Complete {
		t.Errorf("expected COMPLETE, got %q", promise)
	}
}
