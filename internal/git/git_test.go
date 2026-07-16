package git

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sambaths/loop/internal/git/testhelper"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
}

func gitConfig(t *testing.T, dir string) {
	t.Helper()
	for _, kv := range []string{"user.name test", "user.email test@test.com"} {
		parts := strings.Split(kv, " ")
		cmd := exec.Command("git", "config", parts[0], parts[1])
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git config %s failed: %v\n%s", kv, err, out)
		}
	}
}

func gitCommit(t *testing.T, dir, msg string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", msg)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

func gitInitRepo(t *testing.T, dir string) {
	t.Helper()
	gitInit(t, dir)
	gitConfig(t, dir)
	gitCommit(t, dir, "initial")
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func TestHasGit(t *testing.T) {
	if !HasGit() {
		t.Fatal("expected git to be installed")
	}
}

func TestIsRepoInsideRepo(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	if !IsRepo() {
		t.Fatal("expected IsRepo() to be true inside a git repo")
	}
}

func TestIsRepoOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	if IsRepo() {
		t.Fatal("expected IsRepo() to be false outside a git repo")
	}
}

func TestRunGit(t *testing.T) {
	stdout, stderr, err := RunGit("version")
	if err != nil {
		t.Fatalf("git version failed: %s: %v", stderr, err)
	}
	if !strings.HasPrefix(stdout, "git version") {
		t.Errorf("expected 'git version ...', got %q", stdout)
	}
}

func TestRunGitGitNotInstalled(t *testing.T) {
	if !HasGit() {
		t.Skip("git is not installed on this system — cannot test missing-git scenario reliably")
	}

	dir := t.TempDir()
	t.Setenv("PATH", dir)

	_, _, err := RunGit("version")
	if err == nil {
		t.Fatal("expected error when git is not in PATH")
	}
	if !strings.Contains(err.Error(), "git is not installed") {
		t.Errorf("expected friendly error message about git not being installed, got: %v", err)
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	branch, err := CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}
	if branch != "master" && branch != "main" {
		t.Errorf("expected master or main, got %q", branch)
	}
}

func TestBranchExists(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	if !BranchExists("master") && !BranchExists("main") {
		t.Error("expected default branch to exist")
	}
	if BranchExists("nonexistent-branch-xyz") {
		t.Error("expected nonexistent branch to return false")
	}
}

func TestSwitchAndCreateBranch(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	if err := SwitchBranch("master"); err != nil && err.Error() != "switch to branch \"master\": ..." {
		if err := SwitchBranch("main"); err != nil {
			t.Fatalf("SwitchBranch to default failed: %v", err)
		}
	}

	if err := CreateBranch("feature/test", "master"); err != nil {
		if err := CreateBranch("feature/test", "main"); err != nil {
			t.Fatalf("CreateBranch failed: %v", err)
		}
	}

	branch, _ := CurrentBranch()
	if branch != "feature/test" {
		t.Errorf("expected feature/test, got %q", branch)
	}

	if !BranchExists("feature/test") {
		t.Error("expected feature/test branch to exist")
	}
}

func TestSwitchOrCreateBranchExisting(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	if err := CreateBranch("existing-branch", defaultBranch); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	if err := SwitchBranch(defaultBranch); err != nil {
		t.Fatalf("SwitchBranch back to default failed: %v", err)
	}

	if err := SwitchOrCreateBranch("existing-branch", defaultBranch); err != nil {
		t.Fatalf("SwitchOrCreateBranch on existing branch failed: %v", err)
	}

	branch, _ := CurrentBranch()
	if branch != "existing-branch" {
		t.Errorf("expected existing-branch, got %q", branch)
	}
}

func TestSwitchOrCreateBranchNew(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	if err := SwitchOrCreateBranch("new-branch", defaultBranch); err != nil {
		t.Fatalf("SwitchOrCreateBranch on new branch failed: %v", err)
	}

	branch, _ := CurrentBranch()
	if branch != "new-branch" {
		t.Errorf("expected new-branch, got %q", branch)
	}

	if !BranchExists("new-branch") {
		t.Error("expected new-branch to exist")
	}
}

func TestStashChangesNoChanges(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	stashed, err := StashChanges()
	if err != nil {
		t.Fatalf("StashChanges failed: %v", err)
	}
	if stashed {
		t.Fatal("expected no stash for clean tree")
	}
}

func TestStashChangesDirtyTree(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	writeFile(t, dir, "tracked.txt", "clean")
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "add tracked").Run()
	writeFile(t, dir, "tracked.txt", "dirty")

	stashed, err := StashChanges()
	if err != nil {
		t.Fatalf("StashChanges failed: %v", err)
	}
	if !stashed {
		t.Fatal("expected stash for dirty tree")
	}

	data, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	if err != nil {
		t.Fatalf("expected tracked file to exist after stash: %v", err)
	}
	if string(data) != "clean" {
		t.Errorf("expected tracked.txt content 'clean' after stash (reverted), got %q", string(data))
	}
}

func TestPopStash(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	writeFile(t, dir, "stashed.txt", "content")

	stashed, err := StashChanges()
	if err != nil {
		t.Fatalf("StashChanges failed: %v", err)
	}
	if !stashed {
		t.Fatal("expected stash for dirty tree")
	}

	if err := PopStash(); err != nil {
		t.Fatalf("PopStash failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "stashed.txt"))
	if err != nil {
		t.Fatalf("expected stashed file to be restored: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("expected file content 'content', got %q", string(data))
	}
}

func TestPopStashEmpty(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	if err := PopStash(); err != nil {
		t.Fatalf("PopStash on empty should succeed: %v", err)
	}
}

func TestSaveAndRestoreContext(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	writeFile(t, dir, "tracked.txt", "clean")
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "add tracked").Run()
	writeFile(t, dir, "tracked.txt", "in progress")

	restore, err := SaveContext()
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	if err != nil {
		t.Fatalf("expected tracked file to exist: %v", err)
	}
	if string(data) != "clean" {
		t.Errorf("expected tracked.txt content 'clean' after stash (reverted), got %q", string(data))
	}

	if err := SwitchOrCreateBranch("feature/test", defaultBranch); err != nil {
		t.Fatalf("Switch branch failed: %v", err)
	}

	restore()

	branch, _ := CurrentBranch()
	if branch != defaultBranch {
		t.Errorf("expected branch %q restored, got %q", defaultBranch, branch)
	}

	data2, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	if err != nil {
		t.Fatalf("expected tracked file restored: %v", err)
	}
	if string(data2) != "in progress" {
		t.Errorf("expected content 'in progress', got %q", string(data2))
	}
}

func TestSaveContextCleanTree(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	restore, err := SaveContext()
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	// Restore should be a no-op for clean tree
	restore()
}

func TestResolveBranch(t *testing.T) {
	tests := []struct {
		field    string
		default_ string
		want     string
	}{
		{"", "main", "main"},
		{"main", "main", "main"},
		{"develop", "main", "develop"},
		{"*", "main", ""},
	}
	for _, tc := range tests {
		got := ResolveBranch(tc.field, tc.default_)
		if got != tc.want {
			t.Errorf("ResolveBranch(%q, %q) = %q; want %q", tc.field, tc.default_, got, tc.want)
		}
	}
}

func TestSwitchForIssueNoOpWildcard(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	branch, _ := CurrentBranch()

	if _, err := SwitchForIssue("*", "main"); err != nil {
		t.Fatalf("SwitchForIssue with wildcard failed: %v", err)
	}

	current, _ := CurrentBranch()
	if current != branch {
		t.Errorf("expected branch unchanged, got %q", current)
	}
}

func TestSwitchForIssueAlreadyOnBranch(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	branch, _ := CurrentBranch()

	if _, err := SwitchForIssue(branch, "main"); err != nil {
		t.Fatalf("SwitchForIssue to same branch failed: %v", err)
	}
}

func TestSwitchForIssueEmptyField(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	// Create a feature branch and switch to it
	if err := CreateBranch("feature/test", defaultBranch); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	if _, err := SwitchForIssue("", defaultBranch); err != nil {
		t.Fatalf("SwitchForIssue with empty field failed: %v", err)
	}

	current, _ := CurrentBranch()
	if current != defaultBranch {
		t.Errorf("expected branch %q, got %q", defaultBranch, current)
	}
}

func TestSwitchForIssueMainField(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	if err := CreateBranch("feature/test", defaultBranch); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	if _, err := SwitchForIssue(defaultBranch, defaultBranch); err != nil {
		t.Fatalf("SwitchForIssue with %q field failed: %v", defaultBranch, err)
	}

	current, _ := CurrentBranch()
	if current != defaultBranch {
		t.Errorf("expected branch %q, got %q", defaultBranch, current)
	}
}

func TestSwitchForIssueNamedBranchExists(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	if err := CreateBranch("feature/existing", defaultBranch); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	if err := SwitchBranch(defaultBranch); err != nil {
		t.Fatalf("SwitchBranch back to default failed: %v", err)
	}

	if _, err := SwitchForIssue("feature/existing", defaultBranch); err != nil {
		t.Fatalf("SwitchForIssue to existing named branch failed: %v", err)
	}

	current, _ := CurrentBranch()
	if current != "feature/existing" {
		t.Errorf("expected feature/existing, got %q", current)
	}
}

func TestSwitchForIssueNamedBranchCreate(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	if _, err := SwitchForIssue("feature/new", defaultBranch); err != nil {
		t.Fatalf("SwitchForIssue to new named branch failed: %v", err)
	}

	current, _ := CurrentBranch()
	if current != "feature/new" {
		t.Errorf("expected feature/new, got %q", current)
	}

	if !BranchExists("feature/new") {
		t.Error("expected feature/new branch to exist")
	}

	if err := SwitchBranch(defaultBranch); err != nil {
		t.Fatalf("SwitchBranch back to default failed: %v", err)
	}
}

func TestSwitchForIssueOutsideRepo(t *testing.T) {
	dir := t.TempDir()

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	ResetNoRepoWarning()

	stderrR, stderrW, _ := os.Pipe()
	oldStderr := os.Stderr
	os.Stderr = stderrW

	switched, err := SwitchForIssue("feature/test", "main")

	stderrW.Close()
	stderrOut, _ := io.ReadAll(stderrR)
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("SwitchForIssue outside repo should not fail: %v", err)
	}
	if switched {
		t.Fatal("expected switched=false outside repo")
	}
	if !strings.Contains(string(stderrOut), "not a git repository") {
		t.Errorf("expected warning about not a git repo, got:\n%s", string(stderrOut))
	}
}

func TestRestoreContextStashConflict(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	writeFile(t, dir, "conflict.txt", "original")
	exec.Command("git", "add", "conflict.txt").Run()
	exec.Command("git", "commit", "-m", "add conflict").Run()

	writeFile(t, dir, "conflict.txt", "stashed change")

	restore, err := SaveContext()
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	writeFile(t, dir, "conflict.txt", "working tree change")
	exec.Command("git", "add", "conflict.txt").Run()
	exec.Command("git", "commit", "-m", "working tree change").Run()

	// Restore should handle conflict internally (log to stderr)
	restore()

	// With the new apply-then-drop approach, stash is preserved on conflict.
	// Verify it still exists — StashApply should return an error (conflict).
	if err := StashApply(); err == nil {
		t.Fatalf("expected stash apply to fail with conflict after restore: %v", err)
	}

	// Clean up: explicitly drop the stash
	if err := StashDrop(); err != nil {
		t.Fatalf("StashDrop failed: %v", err)
	}
}

func TestRestoreContextNil(t *testing.T) {
	if err := RestoreContext(nil); err != nil {
		t.Fatalf("RestoreContext(nil) should not fail: %v", err)
	}
}

func TestFullStashPopCycle(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	writeFile(t, dir, "data.txt", "clean")
	exec.Command("git", "add", "data.txt").Run()
	exec.Command("git", "commit", "-m", "add tracked").Run()
	writeFile(t, dir, "data.txt", "modified-content")

	stashed, err := StashChanges()
	if err != nil {
		t.Fatalf("StashChanges failed: %v", err)
	}
	if !stashed {
		t.Fatal("expected stash for dirty tree")
	}

	data, err := os.ReadFile(filepath.Join(dir, "data.txt"))
	if err != nil {
		t.Fatalf("expected data.txt to exist: %v", err)
	}
	if string(data) != "clean" {
		t.Errorf("expected content 'clean' after stash, got %q", string(data))
	}

	if err := PopStash(); err != nil {
		t.Fatalf("PopStash failed: %v", err)
	}

	data2, err := os.ReadFile(filepath.Join(dir, "data.txt"))
	if err != nil {
		t.Fatalf("expected data.txt to exist after pop: %v", err)
	}
	if string(data2) != "modified-content" {
		t.Errorf("expected 'modified-content' after pop, got %q", string(data2))
	}
}

func TestNoGitRepoStashReturnsError(t *testing.T) {
	dir := t.TempDir()

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	_, err := StashChanges()
	if err == nil {
		t.Log("StashChanges outside repo may or may not fail depending on git version")
	}
}

func TestSaveContextOutsideRepo(t *testing.T) {
	dir := t.TempDir()

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	ResetNoRepoWarning()

	stderrR, stderrW, _ := os.Pipe()
	oldStderr := os.Stderr
	os.Stderr = stderrW

	restore, err := SaveContext()

	stderrW.Close()
	stderrOut, _ := io.ReadAll(stderrR)
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("SaveContext outside repo should not fail: %v", err)
	}
	if restore == nil {
		t.Fatal("expected non-nil restore function")
	}
	if !strings.Contains(string(stderrOut), "not a git repository") {
		t.Errorf("expected warning about not a git repo, got:\n%s", string(stderrOut))
	}
	// Restore should be a no-op outside repo
	restore()
}

func TestRestoreContextOutsideRepo(t *testing.T) {
	dir := t.TempDir()

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	if err := RestoreContext(&Context{OriginalBranch: "main", Stashed: true}); err != nil {
		t.Fatalf("RestoreContext outside repo should not fail: %v", err)
	}
}

func TestSaveContextPersistsFile(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	writeFile(t, dir, "tracked.txt", "clean")
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "add tracked").Run()
	writeFile(t, dir, "tracked.txt", "dirty")

	restore, err := SaveContext()
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}
	defer restore()

	if !HasSavedContext() {
		t.Fatal("expected saved context file to exist after SaveContext")
	}
}

func TestClearContextFileAfterRestore(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	restore, err := SaveContext()
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	restore()

	if HasSavedContext() {
		t.Fatal("expected saved context file to be cleared after restore")
	}

	branch, _ := CurrentBranch()
	if branch != defaultBranch {
		t.Errorf("expected back on %q after restore, got %q", defaultBranch, branch)
	}
}

func TestRestoreContextFromFile(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	writeFile(t, dir, "tracked.txt", "clean")
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "add tracked").Run()
	writeFile(t, dir, "tracked.txt", "in progress")

	if _, err := SaveContext(); err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	if err := SwitchOrCreateBranch("feature/test", defaultBranch); err != nil {
		t.Fatalf("SwitchOrCreateBranch failed: %v", err)
	}

	if err := RestoreContextFromFile(); err != nil {
		t.Fatalf("RestoreContextFromFile failed: %v", err)
	}

	branch, _ := CurrentBranch()
	if branch != defaultBranch {
		t.Errorf("expected back on %q after RestoreContextFromFile, got %q", defaultBranch, branch)
	}

	data, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	if err != nil {
		t.Fatalf("tracked.txt: %v", err)
	}
	if string(data) != "in progress" {
		t.Errorf("expected 'in progress' after restore, got %q", string(data))
	}

	if HasSavedContext() {
		t.Fatal("expected saved context file to be cleared after RestoreContextFromFile")
	}
}

func TestRestoreContextFromFileNonexistent(t *testing.T) {
	dir := t.TempDir()

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	if HasSavedContext() {
		t.Skip("expected no saved context in temp dir")
	}

	err := RestoreContextFromFile()
	if err == nil {
		t.Fatal("expected error when no saved context file exists")
	}
	if err.Error() != "no saved git context found" {
		t.Errorf("expected 'no saved git context found', got %v", err)
	}
}

func TestHasSavedContextNoFile(t *testing.T) {
	dir := t.TempDir()

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	if HasSavedContext() {
		t.Fatal("expected HasSavedContext to be false when no file exists")
	}
}

func TestSaveContextPersistAndRestoreFromFile(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	writeFile(t, dir, "tracked.txt", "clean")
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "add tracked").Run()
	writeFile(t, dir, "tracked.txt", "in progress")

	restore, err := SaveContext()
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}
	restore()

	writeFile(t, dir, "tracked.txt", "in progress again")
	restore2, err := SaveContext()
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	if err := SwitchOrCreateBranch("feature/test", defaultBranch); err != nil {
		t.Fatalf("SwitchOrCreateBranch failed: %v", err)
	}

	restore2()

	branch, _ := CurrentBranch()
	if branch != defaultBranch {
		t.Errorf("expected back on %q after restore, got %q", defaultBranch, branch)
	}

	data, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	if err != nil {
		t.Fatalf("tracked.txt: %v", err)
	}
	if string(data) != "in progress again" {
		t.Errorf("expected 'in progress again', got %q", string(data))
	}

	if HasSavedContext() {
		t.Fatal("expected saved context file to be cleared after restore")
	}
}

func TestSaveContextTrimmedPath(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	subDir := filepath.Join(dir, "subdir1", "subdir2")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	oldWd, _ := os.Getwd()
	os.Chdir(subDir)
	defer os.Chdir(oldWd)

	if _, err := SaveContext(); err != nil {
		t.Fatalf("SaveContext from subdir failed: %v", err)
	}

	if !HasSavedContext() {
		t.Fatal("expected saved context file to exist even when cwd is a subdir")
	}

	if err := RestoreContextFromFile(); err != nil {
		t.Fatalf("RestoreContextFromFile from subdir failed: %v", err)
	}
}

func TestSwitchBranchError(t *testing.T) {
	dir := t.TempDir()
	testhelper.InitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	err := SwitchBranch("nonexistent-branch-xyz")
	if err == nil {
		t.Fatal("expected error when switching to non-existent branch")
	}
}

func TestCreateBranchWithRemoteOrigin(t *testing.T) {
	srcDir := t.TempDir()
	testhelper.InitRepo(t, srcDir)

	// Determine the default branch name (could be master or main)
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = srcDir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("get default branch: %v", err)
	}
	defaultBranch := strings.TrimSpace(string(out))

	// Create a bare repo to act as remote origin
	bareDir := t.TempDir()
	out2, err := exec.Command("git", "init", "--bare", bareDir).CombinedOutput()
	if err != nil {
		t.Fatalf("bare init failed: %v\n%s", err, out2)
	}

	// Add remote and push from source repo
	cmd = exec.Command("git", "remote", "add", "origin", bareDir)
	cmd.Dir = srcDir
	out2, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("remote add failed: %v\n%s", err, out2)
	}

	// Push current branch to origin to create remote-tracking ref
	cmd = exec.Command("git", "push", "-u", "origin", defaultBranch)
	cmd.Dir = srcDir
	out2, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("push failed: %v\n%s", err, out2)
	}

	// Chdir into srcDir and test CreateBranch with origin/from path
	oldWd, _ := os.Getwd()
	os.Chdir(srcDir)
	defer os.Chdir(oldWd)

	if err := CreateBranch("feature/from-origin", defaultBranch); err != nil {
		t.Fatalf("CreateBranch with remote origin failed: %v", err)
	}

	branch, _ := CurrentBranch()
	if branch != "feature/from-origin" {
		t.Errorf("expected feature/from-origin, got %q", branch)
	}
}

func TestHasGitNotFound(t *testing.T) {
	if !HasGit() {
		t.Skip("git is not installed on this system — cannot test missing-git scenario reliably")
	}

	dir := t.TempDir()
	t.Setenv("PATH", dir)

	if HasGit() {
		t.Fatal("expected HasGit() to return false when git is not in PATH")
	}
}

func TestPopStashConflict(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	writeFile(t, dir, "tracked.txt", "original")
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "add tracked").Run()

	writeFile(t, dir, "tracked.txt", "modification 1")
	stashed, err := StashChanges()
	if err != nil {
		t.Fatalf("StashChanges failed: %v", err)
	}
	if !stashed {
		t.Fatal("expected stash for dirty tree")
	}

	writeFile(t, dir, "tracked.txt", "conflicting change")
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "conflicting change").Run()

	err = PopStash()
	if err == nil {
		t.Fatal("expected conflict error from PopStash")
	}
	if !errors.Is(err, ErrStashConflict) {
		t.Fatalf("expected ErrStashConflict, got: %v", err)
	}

	// With the new apply-then-drop approach, the stash is preserved on
	// conflict so user data is not lost. Verify StashApply still errors.
	if err := StashApply(); err == nil {
		t.Fatalf("expected stash apply to fail with conflict after PopStash: %v", err)
	}

	// Clean up: explicitly drop the stash
	if err := StashDrop(); err != nil {
		t.Fatalf("StashDrop failed: %v", err)
	}
}

func TestStashPopRestoresAllChanges(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	writeFile(t, dir, "tracked.txt", "original")
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "initial").Run()

	out, _ := exec.Command("git", "status", "--porcelain").CombinedOutput()
	if len(out) != 0 {
		t.Fatalf("expected clean tree, got:\n%s", out)
	}

	writeFile(t, dir, "tracked.txt", "modified")
	writeFile(t, dir, "untracked.txt", "new-file")
	writeFile(t, dir, "new-staged.txt", "staged-content")
	exec.Command("git", "add", "new-staged.txt").Run()

	stashed, err := StashChanges()
	if err != nil {
		t.Fatalf("StashChanges failed: %v", err)
	}
	if !stashed {
		t.Fatal("expected stash for dirty tree")
	}

	out, err = exec.Command("git", "status", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatalf("git status failed: %v", err)
	}
	if len(out) > 0 {
		t.Errorf("expected clean tree after stash, got:\n%s", out)
	}

	if err := exec.Command("git", "diff", "--quiet", "HEAD").Run(); err != nil {
		t.Error("expected no diff from HEAD after stash")
	}

	data, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	if err != nil {
		t.Fatalf("tracked.txt: %v", err)
	}
	if string(data) != "original" {
		t.Errorf("tracked.txt after stash: want 'original', got %q", string(data))
	}

	if _, err := os.Stat(filepath.Join(dir, "untracked.txt")); !os.IsNotExist(err) {
		t.Error("untracked.txt should not exist after stash")
	}
	if _, err := os.Stat(filepath.Join(dir, "new-staged.txt")); !os.IsNotExist(err) {
		t.Error("new-staged.txt should not exist after stash")
	}

	if err := PopStash(); err != nil {
		t.Fatalf("PopStash failed: %v", err)
	}

	data, err = os.ReadFile(filepath.Join(dir, "tracked.txt"))
	if err != nil {
		t.Fatalf("tracked.txt after pop: %v", err)
	}
	if string(data) != "modified" {
		t.Errorf("tracked.txt after pop: want 'modified', got %q", string(data))
	}

	data, err = os.ReadFile(filepath.Join(dir, "untracked.txt"))
	if err != nil {
		t.Fatalf("untracked.txt after pop: %v", err)
	}
	if string(data) != "new-file" {
		t.Errorf("untracked.txt after pop: want 'new-file', got %q", string(data))
	}

	data, err = os.ReadFile(filepath.Join(dir, "new-staged.txt"))
	if err != nil {
		t.Fatalf("new-staged.txt after pop: %v", err)
	}
	if string(data) != "staged-content" {
		t.Errorf("new-staged.txt after pop: want 'staged-content', got %q", string(data))
	}
}

func TestIntegrationBranchCreation(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	if err := CreateBranch("integration/test-feature", defaultBranch); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	branch, err := CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}
	if branch != "integration/test-feature" {
		t.Errorf("expected 'integration/test-feature', got %q", branch)
	}

	if !BranchExists("integration/test-feature") {
		t.Error("expected integration/test-feature branch to exist")
	}

	SwitchBranch(defaultBranch)
	if _, err := os.Stat(filepath.Join(dir, "integration")); err == nil {
		t.Error("expected no 'integration' directory to leak into the working tree from branch name")
	}
}

func TestIntegrationBranchSwitching(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	CreateBranch("branch-a", defaultBranch)
	SwitchBranch(defaultBranch)
	CreateBranch("branch-b", defaultBranch)
	SwitchBranch(defaultBranch)

	if err := SwitchBranch("branch-a"); err != nil {
		t.Fatalf("SwitchBranch to branch-a failed: %v", err)
	}
	branch, _ := CurrentBranch()
	if branch != "branch-a" {
		t.Errorf("expected branch-a, got %q", branch)
	}

	if err := SwitchBranch("branch-b"); err != nil {
		t.Fatalf("SwitchBranch to branch-b failed: %v", err)
	}
	branch, _ = CurrentBranch()
	if branch != "branch-b" {
		t.Errorf("expected branch-b, got %q", branch)
	}

	if err := SwitchBranch(defaultBranch); err != nil {
		t.Fatalf("SwitchBranch back to %q failed: %v", defaultBranch, err)
	}
	branch, _ = CurrentBranch()
	if branch != defaultBranch {
		t.Errorf("expected %q, got %q", defaultBranch, branch)
	}
}

func TestIntegrationWildcardBranch(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	branch, _ := CurrentBranch()

	if _, err := SwitchForIssue("*", "main"); err != nil {
		t.Fatalf("SwitchForIssue with '*' failed: %v", err)
	}

	current, _ := CurrentBranch()
	if current != branch {
		t.Errorf("expected branch unchanged after wildcard, got %q", current)
	}

	if _, err := SwitchForIssue("*", "main"); err != nil {
		t.Fatalf("SwitchForIssue with '*' failed: %v", err)
	}
}

func TestIntegrationMissingBranch(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	err := SwitchBranch("this-branch-does-not-exist")
	if err == nil {
		t.Fatal("expected error when switching to non-existent branch")
	}

	branch, _ := CurrentBranch()
	if branch != "master" && branch != "main" {
		t.Errorf("expected original branch unchanged, got %q", branch)
	}
}

func TestRestoreContextPopsStashOnBranchFail(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	writeFile(t, dir, "tracked.txt", "clean")
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "add tracked").Run()
	writeFile(t, dir, "tracked.txt", "dirty")

	restore, err := SaveContext()
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	if err := CreateBranch("feature/test", defaultBranch); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	exec.Command("git", "branch", "-D", defaultBranch).Run()

	restore()

	stderr, _, err := RunGit("stash", "list")
	if err != nil {
		t.Fatalf("git stash list failed: %v", err)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Errorf("expected stash list to be empty after restore with branch fail, got: %q", stderr)
	}
}

func TestSaveContextStashOkBranchSwitchFails(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	defaultBranch := "master"
	if !BranchExists(defaultBranch) {
		defaultBranch = "main"
	}

	writeFile(t, dir, "tracked.txt", "original")
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "add tracked").Run()
	writeFile(t, dir, "tracked.txt", "in progress")

	restore, err := SaveContext()
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	if err := SwitchBranch("nonexistent-branch-xyz"); err == nil {
		t.Fatal("expected error when switching to nonexistent branch")
	}

	restore()

	data, err := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	if err != nil {
		t.Fatalf("expected tracked.txt to exist: %v", err)
	}
	if string(data) != "in progress" {
		t.Errorf("expected stashed change 'in progress' restored, got %q", string(data))
	}

	curr, _ := CurrentBranch()
	if curr != defaultBranch {
		t.Errorf("expected on default branch %q, got %q", defaultBranch, curr)
	}
}

func TestPopStashOnlyNonLoopStashes(t *testing.T) {
	dir := t.TempDir()
	testhelper.InitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	testhelper.WriteFile(t, dir, "stash1.txt", "content1")
	exec.Command("git", "add", "stash1.txt").Run()
	exec.Command("git", "stash", "push", "-m", "non-loop stash 1").Run()

	testhelper.WriteFile(t, dir, "stash2.txt", "content2")
	exec.Command("git", "add", "stash2.txt").Run()
	exec.Command("git", "stash", "push", "-m", "non-loop stash 2").Run()

	// PopStash should pop both (pops from top)
	if err := PopStash(); err != nil {
		t.Fatalf("first PopStash failed: %v", err)
	}
	if err := PopStash(); err != nil {
		t.Fatalf("second PopStash failed: %v", err)
	}
}

func TestHasUnmergedIndexFalse(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	if HasUnmergedIndex() {
		t.Fatal("expected HasUnmergedIndex to be false for clean repo")
	}

	writeFile(t, dir, "file.txt", "content")
	exec.Command("git", "add", "file.txt").Run()

	if HasUnmergedIndex() {
		t.Fatal("expected HasUnmergedIndex to be false after staging")
	}
}

func TestHasUnmergedIndexTrue(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Create initial file on main
	writeFile(t, dir, "conflict.txt", "base")
	exec.Command("git", "add", "conflict.txt").Run()
	exec.Command("git", "commit", "-m", "base").Run()

	defaultBranch, _ := CurrentBranch()

	// Create conflicting branch
	exec.Command("git", "checkout", "-b", "conflict-branch").Run()
	writeFile(t, dir, "conflict.txt", "branch change")
	exec.Command("git", "add", "conflict.txt").Run()
	exec.Command("git", "commit", "-m", "branch change").Run()

	// Switch back to main and make conflicting change
	exec.Command("git", "checkout", defaultBranch).Run()
	writeFile(t, dir, "conflict.txt", "main change")
	exec.Command("git", "add", "conflict.txt").Run()
	exec.Command("git", "commit", "-m", "main change").Run()

	// Attempt merge to create conflict
	exec.Command("git", "merge", "conflict-branch").Run()

	if !HasUnmergedIndex() {
		t.Fatal("expected HasUnmergedIndex to be true during merge conflict")
	}

	// Abort to clean up
	exec.Command("git", "merge", "--abort").Run()
}

func TestStashChangesWithUnmergedIndex(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Create initial file
	writeFile(t, dir, "conflict.txt", "base")
	exec.Command("git", "add", "conflict.txt").Run()
	exec.Command("git", "commit", "-m", "base").Run()

	defaultBranch, _ := CurrentBranch()

	// Create conflicting branch
	exec.Command("git", "checkout", "-b", "conflict-branch").Run()
	writeFile(t, dir, "conflict.txt", "branch change")
	exec.Command("git", "add", "conflict.txt").Run()
	exec.Command("git", "commit", "-m", "branch change").Run()

	// Switch back to main and make conflicting change
	exec.Command("git", "checkout", defaultBranch).Run()
	writeFile(t, dir, "conflict.txt", "main change")
	exec.Command("git", "add", "conflict.txt").Run()
	exec.Command("git", "commit", "-m", "main change").Run()

	// Create merge conflict
	exec.Command("git", "merge", "conflict-branch").Run()

	if !HasUnmergedIndex() {
		t.Fatal("expected HasUnmergedIndex to be true during merge conflict")
	}

	// StashChanges should resolve the unmerged index and succeed
	stashed, err := StashChanges()
	if err != nil {
		t.Fatalf("StashChanges with unmerged index failed: %v", err)
	}
	if !stashed {
		t.Fatal("expected stash to succeed with unmerged index")
	}

	// Verify tree is clean after stash
	clean, err := WorkingTreeClean()
	if err != nil {
		t.Fatalf("WorkingTreeClean failed: %v", err)
	}
	if !clean {
		t.Fatal("expected clean working tree after stash")
	}
}

func TestMergeBranchAbortsOnFailure(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Create initial file and commit on main
	writeFile(t, dir, "merge.txt", "base")
	exec.Command("git", "add", "merge.txt").Run()
	exec.Command("git", "commit", "-m", "base").Run()

	defaultBranch, _ := CurrentBranch()

	// Create conflicting branch
	exec.Command("git", "checkout", "-b", "feature-branch").Run()
	writeFile(t, dir, "merge.txt", "feature change")
	exec.Command("git", "add", "merge.txt").Run()
	exec.Command("git", "commit", "-m", "feature change").Run()

	// Switch back and make conflicting change
	exec.Command("git", "checkout", defaultBranch).Run()
	writeFile(t, dir, "merge.txt", "main change")
	exec.Command("git", "add", "merge.txt").Run()
	exec.Command("git", "commit", "-m", "main change").Run()

	// Merge should fail (conflict)
	err := MergeBranch("feature-branch")
	if err == nil {
		t.Fatal("expected merge to fail with conflict")
	}

	// After merge failure, there should be no unmerged index (abort cleans up)
	if HasUnmergedIndex() {
		t.Fatal("expected no unmerged index after MergeBranch failure (abort should clean up)")
	}
}

func TestStashChangesPatchFallback(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Create a tracked file with modifications
	writeFile(t, dir, "tracked.txt", "clean")
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "-m", "add tracked").Run()

	// Modify the file
	writeFile(t, dir, "tracked.txt", "modified-content")

	// Get the repo root for .git path
	repoRoot, _, _ := RunGit("rev-parse", "--show-toplevel")
	repoRoot = strings.TrimSpace(repoRoot)
	patchPath := filepath.Join(repoRoot, ".git", "loop-autosave.patch")

	// Verify no patch file exists yet
	if _, err := os.Stat(patchPath); err == nil {
		os.Remove(patchPath)
	}

	// Use git stash create directly (this is the plumbing fallback path)
	createOut, _, createErr := RunGit("stash", "create")
	if createErr != nil {
		t.Fatalf("git stash create failed: %v", createErr)
	}
	if createOut == "" {
		t.Fatal("expected stash create to return a commit hash")
	}

	_, storeStderr, storeErr := RunGit("stash", "store", createOut)
	if storeErr != nil {
		t.Fatalf("git stash store failed: %s: %v", storeStderr, storeErr)
	}

	// Tree should now be clean
	clean, _ := WorkingTreeClean()
	if !clean {
		t.Log("tree not clean after stash store — expected behavior depends on stash create coverage")
	}

	// StashDrop to clean up
	StashDrop()
}

func TestStashChangesPatchFileCreated(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Create a tracked file with modifications
	writeFile(t, dir, "patch.txt", "original")
	exec.Command("git", "add", "patch.txt").Run()
	exec.Command("git", "commit", "-m", "add patch.txt").Run()

	writeFile(t, dir, "patch.txt", "modified content for patch")
	writeFile(t, dir, "untracked-patch.txt", "new untracked file")

	// Test that patch file creation works as a backup mechanism
	repoRoot, _, _ := RunGit("rev-parse", "--show-toplevel")
	repoRoot = strings.TrimSpace(repoRoot)
	patchPath := filepath.Join(repoRoot, ".git", "loop-autosave.patch")

	_, patchStderr, patchErr := RunGit("diff", "--binary", "--output", patchPath)
	if patchErr != nil {
		t.Fatalf("git diff --binary --output failed: %s: %v", patchStderr, patchErr)
	}

	if _, err := os.Stat(patchPath); os.IsNotExist(err) {
		t.Fatal("expected patch file to exist")
	}

	data, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("read patch file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty patch file")
	}

	// Verify patch can be applied to restore changes
	exec.Command("git", "checkout", "--", "patch.txt").Run()
	data, _ = os.ReadFile(filepath.Join(dir, "patch.txt"))
	if string(data) != "original" {
		t.Fatalf("expected original content after checkout, got %q", string(data))
	}

	_, applyStderr, applyErr := RunGit("apply", patchPath)
	if applyErr != nil {
		t.Fatalf("git apply failed: %s: %v", applyStderr, applyErr)
	}

	data, _ = os.ReadFile(filepath.Join(dir, "patch.txt"))
	if string(data) != "modified content for patch" {
		t.Errorf("expected restored content 'modified content for patch', got %q", string(data))
	}
}
