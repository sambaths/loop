package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sambaths/loop/internal/config"
)

func TestNewModel(t *testing.T) {
	m := NewModel().(model)
	if m.step != stepIssueDir {
		t.Errorf("expected step stepIssueDir, got %d", m.step)
	}
	if m.title != "loop setup" {
		t.Errorf("expected title 'loop setup', got %q", m.title)
	}
	if len(m.inputs) != 3 {
		t.Errorf("expected 3 inputs, got %d", len(m.inputs))
	}
	if !m.inputs[0].Focused() {
		t.Error("expected first input to be focused")
	}
}

func TestModelQuit(t *testing.T) {
	m := NewModel().(model)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, cmd := m.Update(msg)
	m2 := result.(model)
	if m2.step != stepIssueDir {
		t.Errorf("expected step to stay stepIssueDir, got %d", m2.step)
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestModelCtrlC(t *testing.T) {
	m := NewModel().(model)
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, cmd := m.Update(msg)
	m2 := result.(model)
	if m2.step != stepIssueDir {
		t.Errorf("expected step to stay stepIssueDir, got %d", m2.step)
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestModelAdvanceThroughSteps(t *testing.T) {
	m := NewModel().(model)

	enter := tea.KeyMsg{Type: tea.KeyEnter}

	// Step 1: Issue dir -> Repo
	result, cmd := m.Update(enter)
	m2 := result.(model)
	if m2.step != stepRepo {
		t.Errorf("expected stepRepo, got %d", m2.step)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}

	// Step 2: Repo -> Branch
	result, cmd = m2.Update(enter)
	m3 := result.(model)
	if m3.step != stepBranch {
		t.Errorf("expected stepBranch, got %d", m3.step)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}

	// Step 3: Branch -> Confirm
	result, cmd = m3.Update(enter)
	m4 := result.(model)
	if m4.step != stepConfirm {
		t.Errorf("expected stepConfirm, got %d", m4.step)
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestModelEscGoesBack(t *testing.T) {
	m := NewModel().(model)

	// Advance to repo step first
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	result, _ := m.Update(enter)
	m2 := result.(model)

	// Esc should go back
	esc := tea.KeyMsg{Type: tea.KeyEsc}
	result, _ = m2.Update(esc)
	m3 := result.(model)
	if m3.step != stepIssueDir {
		t.Errorf("expected stepIssueDir, got %d", m3.step)
	}
}

func TestModelEscQuitsOnFirstStep(t *testing.T) {
	m := NewModel().(model)
	esc := tea.KeyMsg{Type: tea.KeyEsc}
	_, cmd := m.Update(esc)
	if cmd == nil {
		t.Error("expected tea.Quit cmd from esc on first step")
	}
}

func TestModelConfirmSavesConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	m := NewModel().(model)
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	// Advance through all input steps
	result, _ := m.Update(enter) // issue dir -> repo
	m2 := result.(model)
	result, _ = m2.Update(enter) // repo -> branch
	m3 := result.(model)
	result, _ = m3.Update(enter) // branch -> confirm
	m4 := result.(model)

	if m4.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m4.step)
	}

	// Enter on confirm should save and go to done
	result, _ = m4.Update(enter)
	m5 := result.(model)
	if m5.step != stepDone {
		t.Errorf("expected stepDone, got %d", m5.step)
	}
}

func TestConfirmWritesConfigFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	m := NewModel().(model)
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	// Advance to repo step
	result, _ := m.Update(enter)
	m2 := result.(model)

	// Advance to branch step (empty repo = local-only)
	result, _ = m2.Update(enter)
	m3 := result.(model)

	// Set custom branch value
	m3.inputs[2].SetValue("develop")

	// Advance to confirm
	result, _ = m3.Update(enter)
	m4 := result.(model)
	if m4.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m4.step)
	}
	if m4.cfg.IssueDir != "docs/issues" {
		t.Errorf("expected IssueDir %q, got %q", "docs/issues", m4.cfg.IssueDir)
	}
	if m4.cfg.Repo != "" {
		t.Errorf("expected empty Repo, got %q", m4.cfg.Repo)
	}
	if m4.cfg.BranchOrigin != "develop" {
		t.Errorf("expected BranchOrigin %q, got %q", "develop", m4.cfg.BranchOrigin)
	}

	// Confirm — write config
	result, _ = m4.Update(enter)
	m5 := result.(model)
	if m5.step != stepDone {
		t.Fatalf("expected stepDone after confirm, got %d", m5.step)
	}

	// Verify config file was written to disk
	loaded, exists, err := config.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !exists {
		t.Fatal("expected config file to exist after confirm")
	}
	if loaded.IssueDir != "docs/issues" {
		t.Errorf("expected IssueDir %q, got %q", "docs/issues", loaded.IssueDir)
	}
	if loaded.Repo != "" {
		t.Errorf("expected empty Repo, got %q", loaded.Repo)
	}
	if loaded.BranchOrigin != "develop" {
		t.Errorf("expected BranchOrigin %q, got %q", "develop", loaded.BranchOrigin)
	}
}

func TestModelView(t *testing.T) {
	m := NewModel().(model)
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	if m.title != "loop setup" {
		t.Errorf("expected title in view, got %q", m.title)
	}
}

func TestModelViewDone(t *testing.T) {
	m := NewModel().(model)
	m.step = stepDone
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view for stepDone")
	}
}

func TestModelViewConfirm(t *testing.T) {
	m := NewModel().(model)
	m.step = stepConfirm
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view for stepConfirm")
	}
}

func TestAdvanceWithoutRepo(t *testing.T) {
	m := NewModel().(model)

	enter := tea.KeyMsg{Type: tea.KeyEnter}

	result, _ := m.Update(enter) // issue dir -> repo
	m2 := result.(model)
	result, _ = m2.Update(enter) // repo -> branch
	m3 := result.(model)

	// Set branch value and advance to confirm
	m3.inputs[2].SetValue("main")
	result, _ = m3.Update(enter) // branch -> confirm
	m4 := result.(model)

	if m4.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m4.step)
	}
}

func TestRepoValidationEmptyRepoAdvances(t *testing.T) {
	m := NewModel().(model)
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	// Advance to repo step
	result, _ := m.Update(enter)
	m2 := result.(model)

	// Keep repo empty and advance
	result, _ = m2.Update(enter)
	m3 := result.(model)

	if m3.step != stepBranch {
		t.Errorf("expected stepBranch with empty repo, got %d", m3.step)
	}
	if m3.errMsg != "" {
		t.Errorf("expected no error with empty repo, got %q", m3.errMsg)
	}
}

func TestRepoValidationStaysOnRepoStep(t *testing.T) {
	m := NewModel().(model)
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	// Advance to repo step
	result, _ := m.Update(enter)
	m2 := result.(model)

	// Set a fake repo
	m2.inputs[1].SetValue("nonexistent-owner/nonexistent-repo")

	// Try to advance — should stay on repo step with error
	result, _ = m2.Update(enter)
	m3 := result.(model)

	if m3.step != stepRepo {
		t.Errorf("expected to stay on stepRepo with invalid repo, got %d", m3.step)
	}
	if m3.errMsg == "" {
		t.Error("expected errMsg to be set for invalid repo")
	}
}

func TestRepoValidationWhitespaceRepoAdvances(t *testing.T) {
	m := NewModel().(model)
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	// Advance to repo step
	result, _ := m.Update(enter)
	m2 := result.(model)

	// Set whitespace-only repo
	m2.inputs[1].SetValue("   ")

	// Advance — should be treated as empty and skip validation
	result, _ = m2.Update(enter)
	m3 := result.(model)

	if m3.step != stepBranch {
		t.Errorf("expected stepBranch with whitespace-only repo, got %d", m3.step)
	}
	if m3.errMsg != "" {
		t.Errorf("expected no error with whitespace-only repo, got %q", m3.errMsg)
	}
	if m3.cfg.Repo != "" {
		t.Errorf("expected cfg.Repo to be empty after trimming whitespace, got %q", m3.cfg.Repo)
	}
}

func TestConfirmViewShowsWarnings(t *testing.T) {
	m := NewModel().(model)
	m.step = stepConfirm
	m.warnings = []string{"Warning: something"}

	view := m.View()
	if !contains(view, "Warning: something") {
		t.Error("expected confirm view to include warning text")
	}
}

func TestConfirmViewShowsError(t *testing.T) {
	m := NewModel().(model)
	m.step = stepConfirm
	m.errMsg = "Project init failed: test error"

	view := m.View()
	if !contains(view, "Project init failed") {
		t.Error("expected confirm view to include error message")
	}
}

func TestConfirmAdvanceWithErrorStaysOnConfirm(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test permission-denied when running as root")
	}
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	// Create a .git dir so findProjectRoot succeeds but remove write
	// permission to make config.Save fail
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	// Make dir read-only so config dir creation fails
	os.Chmod(dir, 0500)
	defer os.Chmod(dir, 0755)

	m := NewModel().(model)
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	// Advance through all input steps
	result, _ := m.Update(enter) // issue dir -> repo
	m2 := result.(model)
	result, _ = m2.Update(enter) // repo -> branch
	m3 := result.(model)
	result, _ = m3.Update(enter) // branch -> confirm
	m4 := result.(model)

	if m4.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m4.step)
	}

	// Enter on confirm should fail to save
	result, _ = m4.Update(enter)
	m5 := result.(model)
	if m5.step != stepConfirm {
		t.Errorf("expected to stay on stepConfirm on error, got %d", m5.step)
	}
	if m5.errMsg == "" {
		t.Error("expected errMsg to be set after save failure")
	}
	if !contains(m5.errMsg, "Error saving config") && !contains(m5.errMsg, "mkdir") && !contains(m5.errMsg, "permission denied") {
		t.Errorf("expected error message about config save failure, got: %q", m5.errMsg)
	}

	// Verify error shows in view
	view := m5.View()
	if !contains(view, m5.errMsg) {
		t.Error("expected view to include error message on confirm step")
	}
}

func TestGitInitWhenNoGitDir(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	m := NewModel().(model)

	// initProject should run git init since no .git or go.mod exists
	err := m.initProject()
	if err != nil {
		t.Fatalf("initProject failed: %v", err)
	}

	// Verify .git was created
	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		t.Error("expected .git directory to exist after git init")
	}
}

func TestInitProjectWhenGitExists(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	m := NewModel().(model)
	err := m.initProject()
	if err != nil {
		t.Fatalf("initProject failed: %v", err)
	}
}

func TestConfirmViewShowsLocalOnlyMode(t *testing.T) {
	m := NewModel().(model)
	m.step = stepConfirm
	m.cfg.Repo = ""

	view := m.View()
	if !contains(view, "local-only mode") {
		t.Error("expected confirm view to show '(local-only mode)' when repo is empty")
	}
}

func TestSuccessViewShowsLocalOnlyMode(t *testing.T) {
	m := NewModel().(model)
	m.step = stepDone
	m.cfg.Repo = ""

	view := m.View()
	if !contains(view, "local-only mode") {
		t.Error("expected success view to show '(local-only mode)' when repo is empty")
	}
}

func TestBranchInputDefaultsToMain(t *testing.T) {
	m := NewModel().(model)
	if m.inputs[2].Value() != config.DefaultBranchOrigin {
		t.Errorf("expected branch input to default to %q, got %q", config.DefaultBranchOrigin, m.inputs[2].Value())
	}
}

func TestBranchInputPlaceholderIsMain(t *testing.T) {
	m := NewModel().(model)
	if m.inputs[2].Placeholder != config.DefaultBranchOrigin {
		t.Errorf("expected branch placeholder to be %q, got %q", config.DefaultBranchOrigin, m.inputs[2].Placeholder)
	}
}

func TestBranchStepEmptyDefaultsToMain(t *testing.T) {
	m := NewModel().(model)
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	result, _ := m.Update(enter) // issue dir -> repo
	m2 := result.(model)
	result, _ = m2.Update(enter) // repo -> branch
	m3 := result.(model)

	// Clear the branch input
	m3.inputs[2].SetValue("")

	result, _ = m3.Update(enter) // branch -> confirm
	m4 := result.(model)

	if m4.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m4.step)
	}
	if m4.cfg.BranchOrigin != config.DefaultBranchOrigin {
		t.Errorf("expected BranchOrigin to default to %q, got %q", config.DefaultBranchOrigin, m4.cfg.BranchOrigin)
	}
}

func TestBranchStepCustomValue(t *testing.T) {
	m := NewModel().(model)
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	result, _ := m.Update(enter) // issue dir -> repo
	m2 := result.(model)
	result, _ = m2.Update(enter) // repo -> branch
	m3 := result.(model)

	m3.inputs[2].SetValue("develop")

	result, _ = m3.Update(enter) // branch -> confirm
	m4 := result.(model)

	if m4.step != stepConfirm {
		t.Fatalf("expected stepConfirm, got %d", m4.step)
	}
	if m4.cfg.BranchOrigin != "develop" {
		t.Errorf("expected BranchOrigin %q, got %q", "develop", m4.cfg.BranchOrigin)
	}
}

func TestBranchStepEscGoesBack(t *testing.T) {
	m := NewModel().(model)
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	result, _ := m.Update(enter) // issue dir -> repo
	m2 := result.(model)
	result, _ = m2.Update(enter) // repo -> branch
	m3 := result.(model)

	if m3.step != stepBranch {
		t.Fatalf("expected stepBranch, got %d", m3.step)
	}

	esc := tea.KeyMsg{Type: tea.KeyEsc}
	result, _ = m3.Update(esc)
	m4 := result.(model)

	if m4.step != stepRepo {
		t.Errorf("expected stepRepo after esc on branch step, got %d", m4.step)
	}
}

func TestConfirmViewShowsConfigPath(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	m := NewModel().(model)
	m.step = stepConfirm

	view := m.View()
	if !contains(view, "Config file:") {
		t.Error("expected confirm view to show config file path")
	}
}

func TestConfirmViewCleanupWhenNoneExist(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	m := NewModel().(model)
	m.step = stepConfirm

	view := m.View()
	if contains(view, "Will remove:") {
		t.Error("expected no 'Will remove' line when no old files exist")
	}
}

func TestSuccessViewShowsPathInstructions(t *testing.T) {
	m := NewModel().(model)
	m.step = stepDone

	view := m.View()
	if !contains(view, "Next steps") {
		t.Error("expected success view to show 'Next steps' section")
	}
	if !contains(view, "loop completion") {
		t.Error("expected success view to mention 'loop completion' for bash completions")
	}
	if !contains(view, "PATH") {
		t.Error("expected success view to mention PATH")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
