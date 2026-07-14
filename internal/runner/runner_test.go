package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sambaths/loop/internal/config"
	"github.com/sambaths/loop/internal/github"
	"github.com/sambaths/loop/internal/issue"
	"github.com/sambaths/loop/internal/pipeline"
)

func TestNewRunner(t *testing.T) {
	p := pipeline.New("/tmp/issues", 5)
	r := New(p)
	if r.Pipeline != p {
		t.Error("expected Pipeline to be set")
	}
}

func TestRunnerDoneImmediately(t *testing.T) {
	p := pipeline.New("/tmp/issues", 0)
	r := New(p)
	if err := r.Run(); err != nil {
		t.Fatalf("expected no error for done pipeline, got: %v", err)
	}
}

func TestRunnerNoIssues(t *testing.T) {
	dir := t.TempDir()
	p := pipeline.New(dir, 5)
	r := New(p)
	err := r.Run()
	if !errors.Is(err, ErrNoIssues) {
		t.Fatalf("expected ErrNoIssues, got: %v", err)
	}
}

func TestRunnerWithIssues(t *testing.T) {
	dir := t.TempDir()
	_, err := issue.Create(dir, issue.StateTestReady, "Test Issue", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	p := pipeline.New(dir, 5)
	r := New(p)
	err = r.Run()
	// Runner should either complete gracefully (issues processed)
	// or fail with a non-ErrNoIssues error (e.g. opencode not found).
	if err != nil && errors.Is(err, ErrNoIssues) {
		t.Fatal("expected either success or a pipeline error, not ErrNoIssues")
	}
}

func TestRunnerExportErrors(t *testing.T) {
	if ErrNoIssues == nil {
		t.Fatal("ErrNoIssues must not be nil")
	}
}

// --- RunLoop tests ---

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

func createTodoIssue(t *testing.T, dir string) {
	t.Helper()
	_, err := issue.Create(dir, issue.StateTodo, "Test Issue",
		"Execution mode: AFK-only\n\n## Blocked by\n\n- None")
	if err != nil {
		t.Fatalf("create todo issue: %v", err)
	}
}

func createTestReadyIssue(t *testing.T, dir string) {
	t.Helper()
	_, err := issue.Create(dir, issue.StateTestReady, "Test Issue",
		"## Blocked by\n\n- None")
	if err != nil {
		t.Fatalf("create test-ready issue: %v", err)
	}
}

func fullBody(title string) string {
	return fmt.Sprintf("# %s\n\nExecution mode: AFK-only\n\n## What to build\n\nTest\n\n## User stories covered\n\nTest\n\n## Acceptance criteria\n\nTest\n\n## UAT plan\n\nTest\n\n## Blocked by\n\n- None\n", title)
}

func TestRunLoopCompleteMovesToTestReady(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 1, 0); err != nil {
		t.Fatalf("RunLoop failed: %v", err)
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

func TestRunLoopTestPassMovesToDone(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTestReadyIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "TEST_PASS", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 1, 0); err != nil {
		t.Fatalf("RunLoop failed: %v", err)
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

func TestRunLoopTestFailMovesToTodo(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTestReadyIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "TEST_FAIL", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 1, 0); err != nil {
		t.Fatalf("RunLoop failed: %v", err)
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

func TestRunLoopNoMoreTasksMovesToQuarantine(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "NO_MORE_TASKS", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 5, 0); err != nil {
		t.Fatalf("RunLoop failed: %v", err)
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
}

func TestRunLoopEmptyDirIsNotError(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = wPipe

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 5, 0); err != nil {
		t.Fatalf("expected nil for empty dir, got: %v", err)
	}

	wPipe.Close()
	os.Stderr = oldStderr

	var buf strings.Builder
	if _, err := io.Copy(&buf, rPipe); err != nil {
		t.Fatal(err)
	}
	rPipe.Close()
	stderr := buf.String()

	if !strings.Contains(stderr, "no issues found in pipeline") {
		t.Errorf("expected message about no issues on stderr, got:\n%s", stderr)
	}
}

func TestRunLoopStreamedEmptyDir(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	var lines []string
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	err := RunLoopStreamed(context.Background(), cfg, 5, 0, true,
		func(line string) { lines = append(lines, line) },
		func(iter, total int, title, role string) {},
	)
	if err != nil {
		t.Fatalf("expected nil for empty dir, got: %v", err)
	}

	output := strings.Join(lines, "\n")
	if !strings.Contains(output, "no issues found in pipeline") {
		t.Errorf("expected message about no issues in streamed output, got:\n%s", output)
	}
}

func createHITLOnlyIssue(t *testing.T, dir string) {
	t.Helper()
	_, err := issue.Create(dir, issue.StateTodo, "HITL Issue",
		"Execution mode: HITL-only\n\n## Blocked by\n\n- None")
	if err != nil {
		t.Fatalf("create HITL-only issue: %v", err)
	}
}

func TestRunLoopHITLOnlyMovesToQuarantine(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createHITLOnlyIssue(t, dir)
	addAndCommit(t, dir)

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 5, 0); err != nil {
		t.Fatalf("RunLoop failed: %v", err)
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

func TestRunLoopStreamedHITLOnlyShowsMessage(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createHITLOnlyIssue(t, dir)
	addAndCommit(t, dir)

	var lines []string
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}

	err := RunLoopStreamed(context.Background(), cfg, 5, 0, true,
		func(line string) { lines = append(lines, line) },
		func(iter, total int, title, role string) {},
	)
	if err != nil {
		t.Fatalf("RunLoopStreamed failed: %v", err)
	}

	output := strings.Join(lines, "\n")
	if !strings.Contains(output, "HITL-only") {
		t.Errorf("expected HITL-only message in streamed output, got:\n%s", output)
	}
	if !strings.Contains(output, "no more tasks") {
		t.Errorf("expected 'no more tasks' message, got:\n%s", output)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Fatalf("expected 1 issue in quarantine, got %d", len(quarantine))
	}
}

func TestRunLoopZeroMaxIter(t *testing.T) {
	dir := t.TempDir()
	createTodoIssue(t, dir)

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 0, 0); err != nil {
		t.Fatalf("expected nil for zero iterations, got: %v", err)
	}
}

func TestRunLoopForceIssueNumNotFound(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	err := RunLoop(cfg, 5, 999)
	if err == nil {
		t.Fatal("expected error for non-existent forced issue number")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestRunLoopForceIssueNumTodo(t *testing.T) {
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

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 1, 42); err != nil {
		t.Fatalf("RunLoop failed: %v", err)
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 1 {
		t.Fatalf("expected 1 issue in test-ready, got %d", len(testReady))
	}
}

func TestRunLoopForceIssueNumTestReady(t *testing.T) {
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

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 1, 42); err != nil {
		t.Fatalf("RunLoop failed: %v", err)
	}

	done, err := issue.List(dir, issue.StateDone)
	if err != nil {
		t.Fatalf("List done: %v", err)
	}
	if len(done) != 1 {
		t.Fatalf("expected 1 issue in done, got %d", len(done))
	}
}

func TestRunLoopForceIssueNumAlreadyDone(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\n"
	_, err := issue.Create(dir, issue.StateDone, "Test Issue", body)
	if err != nil {
		t.Fatalf("create done issue: %v", err)
	}
	addAndCommit(t, dir)

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	err = RunLoop(cfg, 1, 42)
	if err == nil {
		t.Fatal("expected error for already-done issue, got nil")
	}
	if !errors.Is(err, issue.ErrIssueAlreadyDone) {
		t.Errorf("expected ErrIssueAlreadyDone, got: %v", err)
	}
}

func TestRunLoopForceIssueNumTodoHITLOnly(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\n\nExecution mode: HITL-only\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Test Issue", body)
	if err != nil {
		t.Fatalf("create todo issue: %v", err)
	}
	addAndCommit(t, dir)

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	err = RunLoop(cfg, 1, 42)
	if err == nil {
		t.Fatal("expected error for forced HITL-only issue")
	}
	if !errors.Is(err, issue.ErrIssueNonAFK) {
		t.Errorf("expected ErrIssueNonAFK, got: %v", err)
	}
}

func TestRunLoopForceIssueNumTodoCombo(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\n\nExecution mode: Combo\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Test Issue", body)
	if err != nil {
		t.Fatalf("create todo issue: %v", err)
	}
	addAndCommit(t, dir)

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	err = RunLoop(cfg, 1, 42)
	if err == nil {
		t.Fatal("expected error for forced Combo issue")
	}
	if !errors.Is(err, issue.ErrIssueNonAFK) {
		t.Errorf("expected ErrIssueNonAFK, got: %v", err)
	}
}

func TestRunLoopForceIssueNumReadyForAgentHITLOnly(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Test Issue\n\nGitHub: #42\nExecution mode: HITL-only\nStatus: ready-for-agent\n"
	_, err := issue.Create(dir, issue.StateReadyForAgent, "Test Issue", body)
	if err != nil {
		t.Fatalf("create ready-for-agent issue: %v", err)
	}
	addAndCommit(t, dir)

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	err = RunLoop(cfg, 1, 42)
	if err == nil {
		t.Fatal("expected error for forced HITL-only issue in ready-for-agent")
	}
	if !errors.Is(err, issue.ErrIssueNonAFK) {
		t.Errorf("expected ErrIssueNonAFK, got: %v", err)
	}
}

func TestRunLoopNonZeroExitWithValidPromise(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "COMPLETE", 1)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 1, 0); err != nil {
		t.Fatalf("RunLoop should handle non-zero exit with valid promise: %v", err)
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 1 {
		t.Fatalf("expected 1 issue in test-ready, got %d", len(testReady))
	}
}

func TestRunLoopInvalidPromiseMovesToQuarantine(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "TEST_FAIL", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 1, 0); err != nil {
		t.Fatalf("RunLoop should handle invalid promise gracefully, got: %v", err)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Fatalf("expected 1 issue in quarantine, got %d", len(quarantine))
	}
}

func TestRunLoopMaxIterationsEnforced(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "COMPLETE", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 1, 0); err != nil {
		t.Fatalf("RunLoop failed: %v", err)
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 1 {
		t.Fatalf("expected 1 issue in test-ready from 1 iteration, got %d", len(testReady))
	}
}

func TestRunLoopExistingTestReadyFirst(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	_, err := issue.Create(dir, issue.StateTodo, "Todo Issue",
		"Execution mode: AFK-only\n\n## Blocked by\n\n- None")
	if err != nil {
		t.Fatalf("create todo issue: %v", err)
	}

	_, err = issue.Create(dir, issue.StateTestReady, "Test-Me Issue",
		"## Blocked by\n\n- None")
	if err != nil {
		t.Fatalf("create test-ready issue: %v", err)
	}
	addAndCommit(t, dir)

	mockDir := setupMockOpencode(t, "TEST_PASS", 0)
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	if err := RunLoop(cfg, 1, 0); err != nil {
		t.Fatalf("RunLoop failed: %v", err)
	}

	done, err := issue.List(dir, issue.StateDone)
	if err != nil {
		t.Fatalf("List done: %v", err)
	}
	if len(done) != 1 {
		t.Fatalf("expected 1 issue in done, got %d", len(done))
	}

	todo, err := issue.List(dir, issue.StateTodo)
	if err != nil {
		t.Fatalf("List todo: %v", err)
	}
	if len(todo) != 1 {
		t.Fatalf("expected 1 issue to remain in todo, got %d", len(todo))
	}
}

func TestRunLoopWithGitHubLabelSyncFailure(t *testing.T) {
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

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60, Repo: "owner/repo"}
	if err := RunLoop(cfg, 1, 0); err != nil {
		t.Fatalf("RunLoop failed: %v", err)
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

func TestRunLoopStreamedAccumulatesGHFailures(t *testing.T) {
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

	var lines []string
	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60, Repo: "owner/repo"}

	err = RunLoopStreamed(context.Background(), cfg, 1, 0, true,
		func(line string) { lines = append(lines, line) },
		func(iter, total int, title, role string) {},
	)
	if err != nil {
		t.Fatalf("RunLoopStreamed failed: %v", err)
	}

	output := strings.Join(lines, "\n")
	if !strings.Contains(output, "github failures") {
		t.Errorf("expected gh failure report in streamed output, got:\n%s", output)
	}
	if !strings.Contains(output, "gh failure:") {
		t.Errorf("expected gh failure lines in streamed output, got:\n%s", output)
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 1 {
		t.Fatalf("expected 1 issue in test-ready despite gh failure, got %d", len(testReady))
	}
}

func TestRunLoopContextAccumulatesGHFailures(t *testing.T) {
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

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60, Repo: "owner/repo"}
	err = RunLoopContext(context.Background(), cfg, 1, 0, true)

	wPipe.Close()
	os.Stderr = oldStderr

	var buf strings.Builder
	if _, err := io.Copy(&buf, rPipe); err != nil {
		t.Fatal(err)
	}
	rPipe.Close()
	stderr := buf.String()

	if err != nil {
		t.Fatalf("RunLoopContext failed: %v", err)
	}

	if !strings.Contains(stderr, "--- iteration 1 github failures ---") {
		t.Errorf("expected gh failure report on stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "gh failure:") {
		t.Errorf("expected gh failure lines on stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--- end github failures ---") {
		t.Errorf("expected gh failure report footer on stderr, got:\n%s", stderr)
	}

	testReady, err := issue.List(dir, issue.StateTestReady)
	if err != nil {
		t.Fatalf("List test-ready: %v", err)
	}
	if len(testReady) != 1 {
		t.Fatalf("expected 1 issue in test-ready despite gh failure, got %d", len(testReady))
	}
}

func TestRunLoopPreFlightErrorsBlocksIteration(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	body := "# Self-Blocking Issue\n\nGitHub: #1\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- #1\n"
	_, err := issue.Create(dir, issue.StateTodo, "Self-Blocking Issue", body)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	addAndCommit(t, dir)

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	err = RunLoop(cfg, 5, 0)
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

func TestQuarantineAllFilenameCollision(t *testing.T) {
	dir := t.TempDir()

	body := fullBody("Issue B")
	_, err := issue.Create(dir, issue.StateTodo, "Issue B", body)
	if err != nil {
		t.Fatalf("Create Issue B in todo failed: %v", err)
	}

	_, err = issue.Create(dir, issue.StateTestReady, "Issue B", body)
	if err != nil {
		t.Fatalf("Create Issue B in test-ready failed: %v", err)
	}

	ps, err := issue.ScanIssueDir(dir)
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}

	n, dupIssues := issue.QuarantineAll(ps)
	if n != 1 {
		t.Fatalf("expected 1 quarantined file, got %d", n)
	}

	found := false
	for _, di := range dupIssues {
		if strings.Contains(di.Message, "quarantined duplicate filename") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected quarantine message for duplicate filename, got: %v", dupIssues)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Errorf("expected 1 quarantined file on disk, got %d", len(quarantine))
	}
}

func TestQuarantineAllGitHubNumCollision(t *testing.T) {
	dir := t.TempDir()

	bodyA := "# Issue A\n\nGitHub: #42\nExecution mode: AFK-only\n\n## What to build\n\nTest\n\n## User stories covered\n\nTest\n\n## Acceptance criteria\n\nTest\n\n## UAT plan\n\nTest\n\n## Blocked by\n\n- None\n"
	_, err := issue.Create(dir, issue.StateTodo, "Issue A", bodyA)
	if err != nil {
		t.Fatalf("Create Issue A failed: %v", err)
	}

	bodyC := "# Issue C\n\nGitHub: #42\nExecution mode: AFK-only\n\n## What to build\n\nTest\n\n## User stories covered\n\nTest\n\n## Acceptance criteria\n\nTest\n\n## UAT plan\n\nTest\n\n## Blocked by\n\n- None\n"
	_, err = issue.Create(dir, issue.StateTestReady, "Issue C", bodyC)
	if err != nil {
		t.Fatalf("Create Issue C failed: %v", err)
	}

	ps, err := issue.ScanIssueDir(dir)
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}

	n, dupIssues := issue.QuarantineAll(ps)
	if n != 1 {
		t.Fatalf("expected 1 quarantined file, got %d", n)
	}

	found := false
	for _, di := range dupIssues {
		if strings.Contains(di.Message, "duplicate GitHub #42") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected quarantine message for duplicate GitHub #42, got: %v", dupIssues)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Errorf("expected 1 quarantined file on disk, got %d", len(quarantine))
	}
}

func TestQuarantineAllTitleCollision(t *testing.T) {
	dir := t.TempDir()

	body := fullBody("Same Title")
	_, err := issue.Create(dir, issue.StateTodo, "Same Title", body)
	if err != nil {
		t.Fatalf("Create Same Title in todo failed: %v", err)
	}

	secondPath := filepath.Join(dir, "test-ready", "different-name.md")
	if err := os.MkdirAll(filepath.Dir(secondPath), 0755); err != nil {
		t.Fatalf("mkdir test-ready: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte(body), 0644); err != nil {
		t.Fatalf("write second file: %v", err)
	}

	ps, err := issue.ScanIssueDir(dir)
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}

	n, dupIssues := issue.QuarantineAll(ps)
	if n != 1 {
		t.Fatalf("expected 1 quarantined file, got %d", n)
	}

	found := false
	for _, di := range dupIssues {
		if strings.Contains(di.Message, "quarantined duplicate title") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected quarantine message for duplicate title, got: %v", dupIssues)
	}

	quarantine, err := issue.List(dir, issue.StateQuarantine)
	if err != nil {
		t.Fatalf("List quarantine: %v", err)
	}
	if len(quarantine) != 1 {
		t.Errorf("expected 1 quarantined file on disk, got %d", len(quarantine))
	}
}

func TestRunLoopAgentNotFoundReturnsError(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	defer chdir(t, dir)()

	createTodoIssue(t, dir)
	addAndCommit(t, dir)

	emptyPath := t.TempDir()
	t.Setenv("PATH", emptyPath)

	cfg := &config.Config{IssueDir: dir, AgentTimeout: 60}
	err := RunLoop(cfg, 1, 0)
	if err == nil {
		t.Fatal("expected error when opencode is not on PATH")
	}
}

func TestEnsureGHAuthNoRepo(t *testing.T) {
	github.ResetAuthCheck()

	cfg := &config.Config{Repo: ""}
	ensureGHAuth(cfg)
	if cfg.Repo != "" {
		t.Error("ensureGHAuth should not change empty repo")
	}
}

func TestEnsureGHAuthAuthFailure(t *testing.T) {
	github.ResetAuthCheck()

	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
	echo "not logged in" >&2
	exit 1
fi
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	cfg := &config.Config{Repo: "owner/repo"}
	ensureGHAuth(cfg)
	if cfg.Repo != "" {
		t.Error("expected repo to be cleared when gh auth fails")
	}
}

func TestEnsureGHAuthAuthSuccess(t *testing.T) {
	github.ResetAuthCheck()

	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
	exit 0
fi
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	cfg := &config.Config{Repo: "owner/repo"}
	ensureGHAuth(cfg)
	if cfg.Repo != "owner/repo" {
		t.Errorf("expected repo to remain 'owner/repo', got %q", cfg.Repo)
	}
}
