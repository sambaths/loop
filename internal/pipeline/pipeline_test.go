package pipeline

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sambaths/loop/internal/github"
	"github.com/sambaths/loop/internal/issue"
)

func TestNewPipeline(t *testing.T) {
	p := New("/tmp/issues", 5)
	if p.IssueDir != "/tmp/issues" {
		t.Errorf("expected IssueDir /tmp/issues, got %q", p.IssueDir)
	}
	if p.MaxIterations != 5 {
		t.Errorf("expected MaxIterations 5, got %d", p.MaxIterations)
	}
	if p.Iteration != 0 {
		t.Errorf("expected Iteration 0, got %d", p.Iteration)
	}
	if p.AgentTimeout != defaultAgentTimeout {
		t.Errorf("expected AgentTimeout %v, got %v", defaultAgentTimeout, p.AgentTimeout)
	}
}

func TestPipelineDone(t *testing.T) {
	p := New("/tmp/issues", 3)
	if p.Done() {
		t.Error("expected not done for new pipeline")
	}
	p.Iteration = 3
	if !p.Done() {
		t.Error("expected done after max iterations")
	}
}

func TestPipelineIterateNoIssues(t *testing.T) {
	dir := t.TempDir()
	p := New(dir, 5)
	err := p.Iterate()
	if err == nil {
		t.Fatal("expected error for empty issue directory")
	}
}

func TestPipelineHITLOnlyMovesToQuarantine(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nExecution mode: HITL-only\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Test Issue", body)
	if err != nil {
		t.Fatalf("create todo issue: %v", err)
	}
	addAndCommit(t, dir)

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate failed: %v", err)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Fatalf("expected 1 issue in quarantine, got %d", len(quarantine))
	}
	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 0 {
		t.Fatalf("expected 0 issues in todo, got %d", len(todo))
	}
}

func TestPipelineIssueListPopulated(t *testing.T) {
	dir := t.TempDir()

	_, err := issue.Create(dir, issue.StateTestReady, "Alpha", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = issue.Create(dir, issue.StateTestReady, "Beta", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	issues, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	expected := "Alpha"
	if issues[0].Title != expected {
		t.Errorf("expected first issue title %q, got %q", expected, issues[0].Title)
	}
}

func TestPipelineIterateRunsAgent(t *testing.T) {
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode not found in PATH, skipping integration test")
	}

	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	_, err := issue.Create(dir, issue.StateTestReady, "Integration Test", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	addAndCommit(t, dir)

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate failed: %v", err)
	}
}

func TestPipelineIterateExhausted(t *testing.T) {
	p := New("/tmp", 2)
	p.Iteration = 2
	err := p.Iterate()
	if err == nil {
		t.Fatal("expected error for exhausted pipeline")
	}
}

func TestPipelineMultipleIssuesPicksFirst(t *testing.T) {
	dir := t.TempDir()
	issue.Create(dir, issue.StateTestReady, "Alpha", "Body")
	issue.Create(dir, issue.StateTestReady, "Beta", "Body")

	p := New(dir, 1)
	issues, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected issues to exist")
	}
	p.CurrentIssue = &issues[0]
	p.Iteration++
	if p.CurrentIssue.Title != "Alpha" {
		t.Errorf("expected first issue 'Alpha', got %q", p.CurrentIssue.Title)
	}
	if p.Iteration != 1 {
		t.Errorf("expected iteration 1, got %d", p.Iteration)
	}
}

func TestPipelineMaxIterations(t *testing.T) {
	p := New("/tmp", 0)
	if !p.Done() {
		t.Error("expected pipeline with max 0 to be done immediately")
	}
}

func TestPipelineDefaultBranchOrigin(t *testing.T) {
	p := New("/tmp/issues", 5)
	if p.BranchOrigin != "main" {
		t.Errorf("expected BranchOrigin default 'main', got %q", p.BranchOrigin)
	}
}

func TestPipelineCustomAgentTimeout(t *testing.T) {
	p := New("/tmp/issues", 5)
	p.AgentTimeout = 10 * time.Minute
	if p.AgentTimeout != 10*time.Minute {
		t.Errorf("expected AgentTimeout 10m, got %v", p.AgentTimeout)
	}
}

func TestPipelineCustomBranchOrigin(t *testing.T) {
	p := New("/tmp/issues", 5)
	p.BranchOrigin = "develop"
	if p.BranchOrigin != "develop" {
		t.Errorf("expected BranchOrigin 'develop', got %q", p.BranchOrigin)
	}
}

func TestStateFromDirTodo(t *testing.T) {
	got := stateFromDir("/tmp/issues", "/tmp/issues")
	if got != issue.StateTodo {
		t.Errorf("stateFromDir = %q, want %q", got, issue.StateTodo)
	}
}

func TestStateFromDirTestReady(t *testing.T) {
	got := stateFromDir("/tmp/issues/test-ready", "/tmp/issues")
	if got != issue.StateTestReady {
		t.Errorf("stateFromDir = %q, want %q", got, issue.StateTestReady)
	}
}

func TestStateFromDirDone(t *testing.T) {
	got := stateFromDir("/tmp/issues/done", "/tmp/issues")
	if got != issue.StateDone {
		t.Errorf("stateFromDir = %q, want %q", got, issue.StateDone)
	}
}

func TestStateFromDirQuarantine(t *testing.T) {
	got := stateFromDir("/tmp/issues/.quarantine", "/tmp/issues")
	if got != issue.StateQuarantine {
		t.Errorf("stateFromDir = %q, want %q", got, issue.StateQuarantine)
	}
}

func TestStateFromDirUnknown(t *testing.T) {
	got := stateFromDir("/tmp/issues/unknown", "/tmp/issues")
	if got != issue.StateTodo {
		t.Errorf("stateFromDir = %q, want %q", got, issue.StateTodo)
	}
}

func TestStateFromDirIssuesDirSelf(t *testing.T) {
	got := stateFromDir("/tmp/my-issues", "/tmp/my-issues")
	if got != issue.StateTodo {
		t.Errorf("stateFromDir = %q, want %q", got, issue.StateTodo)
	}
}

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

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return func() { os.Chdir(oldWd) }
}

func addAndCommit(t *testing.T, dir string) {
	t.Helper()
	exec.Command("git", "add", "-A").Run()
	exec.Command("git", "commit", "-m", "add issue").Run()
}

func TestPipelineIterateOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	defer chdir(t, dir)()

	_, err := issue.Create(dir, issue.StateTestReady, "Test Issue", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	p := New(dir, 5)
	err = p.Iterate()
	// The error (if any) must NOT be from git operations
	if err != nil && strings.Contains(err.Error(), "git") {
		t.Fatalf("expected non-git error, got git-related error: %v", err)
	}
}

// setupMockOpencode creates a temp dir with a mock opencode binary that outputs
// the given promise marker and exits with the given code. It also sets
// MOCK_OPENDCODE_RESULT and MOCK_OPENDCODE_EXIT_CODE for the test process.
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

// createTodoIssue creates a minimal todo issue that SelectIssue will pick
// (requires Execution mode: AFK-only).
func createTodoIssue(t *testing.T, dir string) {
	t.Helper()
	_, err := issue.Create(dir, issue.StateTodo, "Test Issue",
		"Execution mode: AFK-only\n\n## Blocked by\n\n- None")
	if err != nil {
		t.Fatalf("create todo issue: %v", err)
	}
}

// createTestReadyIssue creates a minimal test-ready issue that SelectIssue will pick.
func createTestReadyIssue(t *testing.T, dir string) {
	t.Helper()
	_, err := issue.Create(dir, issue.StateTestReady, "Test Issue",
		"## Blocked by\n\n- None")
	if err != nil {
		t.Fatalf("create test-ready issue: %v", err)
	}
}

func TestPipelineIterateCompleteMovesToTestReady(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate failed: %v", err)
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 1 {
		t.Fatalf("expected 1 issue in test-ready, got %d", len(testReady))
	}

	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 0 {
		t.Fatalf("expected 0 issues in todo, got %d", len(todo))
	}
}

func TestPipelineIterateTestPassMovesToDone(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTestReadyIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "TEST_PASS", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate failed: %v", err)
	}

	done, err := issue.List(dir, issue.StateDone)
	if err != nil {
		t.Fatalf("List done: %v", err)
	}
	if len(done) != 1 {
		t.Fatalf("expected 1 issue in done, got %d", len(done))
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 0 {
		t.Fatalf("expected 0 issues in test-ready, got %d", len(testReady))
	}
}

func TestPipelineIterateTestFailMovesToTodo(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTestReadyIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "TEST_FAIL", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate failed: %v", err)
	}

	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 1 {
		t.Fatalf("expected 1 issue in todo, got %d", len(todo))
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 0 {
		t.Fatalf("expected 0 issues in test-ready, got %d", len(testReady))
	}
}

func TestPipelineIterateTestFailStripsSections(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nExecution mode: AFK-only\n\n## What to build\n\nTest content\n\n## Test Results\n\nAll tests passed\n"
	_, err := issue.Create(dir, issue.StateTestReady, "Test Issue", body)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "TEST_FAIL", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate failed: %v", err)
	}

	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 1 {
		t.Fatalf("expected 1 issue in todo, got %d", len(todo))
	}

	data, err := os.ReadFile(todo[0].FilePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "## Test Results") {
		t.Error("expected ## Test Results to be stripped after TEST_FAIL")
	}
	if !strings.Contains(content, "## What to build") {
		t.Error("expected ## What to build to be preserved after TEST_FAIL")
	}
}

func TestPipelineIterateNoMoreTasksMovesToQuarantine(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "NO_MORE_TASKS", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate failed: %v", err)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Fatalf("expected 1 issue in quarantine, got %d", len(quarantine))
	}

	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 0 {
		t.Fatalf("expected 0 issues in todo, got %d", len(todo))
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 0 {
		t.Fatalf("expected 0 issues in test-ready, got %d", len(testReady))
	}

	done, err := issue.List(dir, issue.StateDone)
	if err != nil {
		t.Fatalf("List done: %v", err)
	}
	if len(done) != 0 {
		t.Fatalf("expected 0 issues in done, got %d", len(done))
	}
}

func TestPipelineIterateNonZeroExitMovesToQuarantine(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "INVALID", 1)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate failed: %v", err)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Fatalf("expected 1 issue in quarantine, got %d", len(quarantine))
	}

	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 0 {
		t.Fatalf("expected 0 issues in todo, got %d", len(todo))
	}
}

func TestPipelineIterateInvalidPromiseMovesToQuarantine(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTestReadyIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "INVALID", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate failed: %v", err)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Fatalf("expected 1 issue in quarantine, got %d", len(quarantine))
	}
}

func TestPipelineIterateAgentNotFoundMovesToQuarantine(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	emptyPath := t.TempDir()
	t.Setenv("PATH", emptyPath)

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate should not fail on agent not found: %v", err)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Fatalf("expected 1 issue in quarantine after agent not found, got %d", len(quarantine))
	}

	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 0 {
		t.Fatalf("expected 0 issues in todo, got %d", len(todo))
	}
}

func TestPipelineIterateTransitionFailureMovesToQuarantine(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTestReadyIssue(t, dir)
	addAndCommit(t, dir)

	// COMPLETE from a test-ready issue (test role) triggers ComputeTransition error.
	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate should not fail on transition error: %v", err)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Fatalf("expected 1 issue in quarantine after transition failure, got %d", len(quarantine))
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 0 {
		t.Fatalf("expected 0 issues in test-ready, got %d", len(testReady))
	}
}

func TestPipelineIteratePreFlightErrorsBlocksIteration(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Self-Blocking Issue\n\nGitHub: #1\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- #1\n"
	_, err := issue.Create(dir, issue.StateTodo, "Self-Blocking Issue", body)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	addAndCommit(t, dir)

	p := New(dir, 5)
	err = p.Iterate()
	if !errors.Is(err, issue.ErrPreFlightFailed) {
		t.Fatalf("expected ErrPreFlightFailed, got: %v", err)
	}

	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 1 {
		t.Fatalf("expected 1 issue in todo, got %d", len(todo))
	}

	done, err := issue.List(dir, issue.StateDone)
	if err != nil {
		t.Fatalf("List done: %v", err)
	}
	if len(done) != 0 {
		t.Fatalf("expected 0 issues in done, got %d", len(done))
	}
}

func TestPipelineIteratePreFlightWarningsAllowProceed(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTestReadyIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "TEST_PASS", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	err := p.Iterate()
	if err != nil {
		t.Fatalf("expected iteration to proceed with warnings, got: %v", err)
	}

	done, err := issue.List(dir, issue.StateDone)
	if err != nil {
		t.Fatalf("List done: %v", err)
	}
	if len(done) != 1 {
		t.Fatalf("expected 1 issue in done, got %d", len(done))
	}
}

func TestPipelineIterateReportsGHFailures(t *testing.T) {
	github.ResetAuthCheck()

	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Test Issue", body)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "COMPLETE", 0)

	mockGh := filepath.Join(mockDir, "gh")
	ghScript := `#!/bin/bash
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
	exit 0
fi
exit 1
`
	if err := os.WriteFile(mockGh, []byte(ghScript), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	// Capture stderr
	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = wPipe

	p := New(dir, 5)
	p.Repo = "owner/repo"

	if err := p.Iterate(); err != nil {
		wPipe.Close()
		os.Stderr = oldStderr
		t.Fatalf("Iterate failed: %v", err)
	}

	wPipe.Close()
	os.Stderr = oldStderr

	var buf strings.Builder
	if _, err := io.Copy(&buf, rPipe); err != nil {
		t.Fatal(err)
	}
	rPipe.Close()
	stderr := buf.String()

	if !strings.Contains(stderr, "--- iteration 1 github failures ---") {
		t.Errorf("expected gh failures report header, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "gh failure:") {
		t.Errorf("expected gh failures in output, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--- end github failures ---") {
		t.Errorf("expected gh failures report footer, got:\n%s", stderr)
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 1 {
		t.Fatalf("expected 1 issue in test-ready despite gh failure, got %d", len(testReady))
	}

	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 0 {
		t.Fatalf("expected 0 issues in todo, got %d", len(todo))
	}
}

func TestPipelineIterateInvalidRolePromiseMovesToQuarantine(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	// Implement role with TEST_FAIL promise triggers validateRolePromise error.
	mockDir := setupMockOpencode(t, "TEST_FAIL", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate should not fail on transition error: %v", err)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Fatalf("expected 1 issue in quarantine after invalid role promise, got %d", len(quarantine))
	}

	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 0 {
		t.Fatalf("expected 0 issues in todo, got %d", len(todo))
	}
}

func TestPipelineForceIssueNumTodo(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Test Issue", body)
	if err != nil {
		t.Fatalf("create todo issue: %v", err)
	}
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	p.ForceIssueNum = 42
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate with forced issue failed: %v", err)
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 1 {
		t.Fatalf("expected 1 issue in test-ready after forced implementation, got %d", len(testReady))
	}
}

func TestPipelineForceIssueNumTestReady(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTestReady, "Test Issue", body)
	if err != nil {
		t.Fatalf("create test-ready issue: %v", err)
	}
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "TEST_PASS", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	p.ForceIssueNum = 42
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate with forced issue failed: %v", err)
	}

	done, err := issue.List(dir, issue.StateDone)
	if err != nil {
		t.Fatalf("List done: %v", err)
	}
	if len(done) != 1 {
		t.Fatalf("expected 1 issue in done after forced test, got %d", len(done))
	}
}

func TestPipelineForceIssueNumNotFound(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Test Issue", body)
	if err != nil {
		t.Fatalf("create todo issue: %v", err)
	}
	addAndCommit(t, dir)

	p := New(dir, 5)
	p.ForceIssueNum = 999
	err = p.Iterate()
	if err == nil {
		t.Fatal("expected error for non-existent forced issue number")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestPipelineForceIssueNumAlreadyDone(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\n"
	_, err := issue.Create(dir, issue.StateDone, "Test Issue", body)
	if err != nil {
		t.Fatalf("create done issue: %v", err)
	}
	addAndCommit(t, dir)

	p := New(dir, 5)
	p.ForceIssueNum = 42
	err = p.Iterate()
	if err == nil {
		t.Fatal("expected error for already-done forced issue")
	}
	if !errors.Is(err, issue.ErrIssueAlreadyDone) {
		t.Errorf("expected ErrIssueAlreadyDone, got: %v", err)
	}
}

func TestPipelineForceIssueNumTodoHITLOnly(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\n\nExecution mode: HITL-only\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Test Issue", body)
	if err != nil {
		t.Fatalf("create todo issue: %v", err)
	}
	addAndCommit(t, dir)

	p := New(dir, 5)
	p.ForceIssueNum = 42
	err = p.Iterate()
	if err == nil {
		t.Fatal("expected error for forced HITL-only issue")
	}
	if !errors.Is(err, issue.ErrIssueNonAFK) {
		t.Errorf("expected ErrIssueNonAFK, got: %v", err)
	}
}

func TestPipelineForceIssueNumTodoCombo(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\n\nExecution mode: Combo\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Test Issue", body)
	if err != nil {
		t.Fatalf("create todo issue: %v", err)
	}
	addAndCommit(t, dir)

	p := New(dir, 5)
	p.ForceIssueNum = 42
	err = p.Iterate()
	if err == nil {
		t.Fatal("expected error for forced Combo issue")
	}
	if !errors.Is(err, issue.ErrIssueNonAFK) {
		t.Errorf("expected ErrIssueNonAFK, got: %v", err)
	}
}

func TestPipelineForceIssueNumReadyForAgentHITLOnly(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\nExecution mode: HITL-only\nStatus: ready-for-agent\n"
	_, err := issue.Create(dir, issue.StateReadyForAgent, "Test Issue", body)
	if err != nil {
		t.Fatalf("create ready-for-agent issue: %v", err)
	}
	addAndCommit(t, dir)

	p := New(dir, 5)
	p.ForceIssueNum = 42
	err = p.Iterate()
	if err == nil {
		t.Fatal("expected error for forced HITL-only issue in ready-for-agent")
	}
	if !errors.Is(err, issue.ErrIssueNonAFK) {
		t.Errorf("expected ErrIssueNonAFK, got: %v", err)
	}
}

func TestPipelineForceIssueNumSkippedOnLaterIterations(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Test Issue", body)
	if err != nil {
		t.Fatalf("create todo issue: %v", err)
	}
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	p.Iteration = 1
	p.ForceIssueNum = 42
	err = p.Iterate()
	if err != nil && !errors.Is(err, issue.ErrNoIssues) {
		t.Fatalf("expected no error or ErrNoIssues for forced issue on iteration 1+, got: %v", err)
	}
}

func TestPipelinePostAgentDuplicateGitHubNum(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	bodyA := "# Issue A\n\nGitHub: #42\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Issue A", bodyA)
	if err != nil {
		t.Fatalf("Create A failed: %v", err)
	}
	addAndCommit(t, dir)

	mockDir := t.TempDir()
	mockPath := filepath.Join(mockDir, "opencode")
	script := fmt.Sprintf("#!/bin/bash\ncat > /dev/null\ncat > %s/issue-duplicate.md << 'EOF'\n# Issue Duplicate\n\nGitHub: #42\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- None\nEOF\necho \"__LOOP_RESULT__\"\necho \"COMPLETE\"\necho \"__LOOP_RESULT_END__\"\n", dir)
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock opencode: %v", err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = wPipe

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		wPipe.Close()
		os.Stderr = oldStderr
		t.Fatalf("Iterate failed: %v", err)
	}

	wPipe.Close()
	os.Stderr = oldStderr

	var buf strings.Builder
	if _, err := io.Copy(&buf, rPipe); err != nil {
		t.Fatal(err)
	}
	rPipe.Close()
	stderr := buf.String()

	if !strings.Contains(stderr, "quarantined duplicate GitHub #42") {
		t.Errorf("expected quarantine message for duplicate GitHub #42 on stderr, got:\n%s", stderr)
	}

	if !strings.Contains(stderr, "was quarantined as duplicate") {
		t.Errorf("expected 'was quarantined as duplicate' message, got:\n%s", stderr)
	}

	// The original issue was quarantined as a duplicate; the agent-created
	// file (canonical) stays in todo since the transition was skipped.
	todoIssues, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todoIssues) != 1 {
		t.Fatalf("expected 1 issue in todo (agent-created canonical), got %d", len(todoIssues))
	}
	if todoIssues[0].Title != "Issue Duplicate" {
		t.Errorf("expected todo title to be %q, got %q", "Issue Duplicate", todoIssues[0].Title)
	}

	quarantineIssues, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantineIssues) != 1 {
		t.Fatalf("expected 1 issue in quarantine, got %d", len(quarantineIssues))
	}
	if !strings.Contains(quarantineIssues[0].Title, "Issue A") {
		t.Errorf("expected quarantined issue title to contain %q, got %q", "Issue A", quarantineIssues[0].Title)
	}
}
func TestPipelinePostAgentDuplicateScan(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	bodyA := "# Issue A\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Issue A", bodyA)
	if err != nil {
		t.Fatalf("Create A failed: %v", err)
	}

	bodyB := "# Issue B\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- None\n"
	issB, err := issue.Create(dir, issue.StateTodo, "Issue B", bodyB)
	if err != nil {
		t.Fatalf("Create B failed: %v", err)
	}
	addAndCommit(t, dir)

	mockDir := t.TempDir()
	dupDir := filepath.Join(dir, "test-ready")
	dupFile := filepath.Join(dupDir, filepath.Base(issB.FilePath))

	script := fmt.Sprintf("#!/bin/bash\ncat > /dev/null\nmkdir -p %s\necho 'duplicate of issue B' > %s\necho \"__LOOP_RESULT__\"\necho \"COMPLETE\"\necho \"__LOOP_RESULT_END__\"\n", dupDir, dupFile)

	mockPath := filepath.Join(mockDir, "opencode")
	if err := os.WriteFile(mockPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock opencode: %v", err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	p := New(dir, 5)
	if err := p.Iterate(); err != nil {
		t.Fatalf("Iterate failed: %v", err)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Fatalf("expected 1 quarantined file, got %d: %v", len(quarantine), quarantine)
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 1 {
		t.Fatalf("expected 1 issue in test-ready (Issue A), got %d: %v", len(testReady), testReady)
	}

	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 0 {
		t.Fatalf("expected 0 issues in todo, got %d: %v", len(todo), todo)
	}
}
