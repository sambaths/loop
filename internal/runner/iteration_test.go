package runner

import (
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

func TestRunIterationInvalidPromise(t *testing.T) {
	dir := t.TempDir()
	issueFile := createIssueFile(t, dir, issue.StateTodo, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencode(t, "INVALID", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	_, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err == nil {
		t.Fatal("expected error for invalid promise")
	}
	if !strings.Contains(err.Error(), "no valid promise") {
		t.Errorf("expected 'no valid promise' error, got: %v", err)
	}
}

func TestRunIterationNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	issueFile := createIssueFile(t, dir, issue.StateTodo, "Test Issue")
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	mockDir := setupMockOpencode(t, "INVALID", 1)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	_, err := RunIteration(cfg, issueFile, issue.RoleImplement)
	if err == nil {
		t.Fatal("expected error for non-zero exit with invalid promise")
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
