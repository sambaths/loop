package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sambaths/loop/internal/config"
	"github.com/sambaths/loop/internal/git/testhelper"
	"github.com/sambaths/loop/internal/github"
	"github.com/sambaths/loop/internal/issue"
	"github.com/sambaths/loop/internal/tui/dashboard"
	"github.com/sambaths/loop/internal/tui/status"
)

func TestParseArgsHelpFlag(t *testing.T) {
	cmd, code := parseArgs([]string{"--help"})
	if cmd != cmdHelp {
		t.Errorf("expected cmdHelp, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsHelpShort(t *testing.T) {
	cmd, code := parseArgs([]string{"-h"})
	if cmd != cmdHelp {
		t.Errorf("expected cmdHelp, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsHelpSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"help"})
	if cmd != cmdHelp {
		t.Errorf("expected cmdHelp, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsVersion(t *testing.T) {
	cmd, code := parseArgs([]string{"--version"})
	if cmd != cmdVersion {
		t.Errorf("expected cmdVersion, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsUnknownSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"foo"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsEmpty(t *testing.T) {
	cmd, code := parseArgs([]string{})
	if cmd != cmdTUI {
		t.Errorf("expected cmdTUI, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsSetupSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"setup"})
	if cmd != cmdSetup {
		t.Errorf("expected cmdSetup, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsRunSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"run", "5"})
	if cmd != cmdRun {
		t.Errorf("expected cmdRun, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if runN != 5 {
		t.Errorf("expected runN=5, got %d", runN)
	}
}

func TestParseArgsRunNoArg(t *testing.T) {
	cmd, code := parseArgs([]string{"run"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsRunInvalidArg(t *testing.T) {
	cmd, code := parseArgs([]string{"run", "abc"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsRunZero(t *testing.T) {
	cmd, code := parseArgs([]string{"run", "0"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsRunNegative(t *testing.T) {
	cmd, code := parseArgs([]string{"run", "-1"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsStatusSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"status"})
	if cmd != cmdStatus {
		t.Errorf("expected cmdStatus, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsHelpOverridesSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"--help", "run"})
	if cmd != cmdHelp {
		t.Errorf("expected cmdHelp, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestDashboardQuit(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m2 := result.(dashboard.Model)

	if m2.View() != "" {
		t.Error("expected empty view after quit")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestDashboardCtrlC(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m2 := result.(dashboard.Model)

	if m2.View() != "" {
		t.Error("expected empty view after quit")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestDashboardView(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	view := m.View()
	if !strings.Contains(view, "loop v") {
		t.Errorf("expected view to contain version header, got:\n%s", view)
	}
	if !strings.Contains(view, "quit") {
		t.Error("expected view to contain help text")
	}
}

func TestDashboardViewQuit(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m2 := result.(dashboard.Model)
	view := m2.View()
	if view != "" {
		t.Error("expected empty view when quit is true")
	}
}

func TestDashboardStartsAtOverview(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	view := m.View()
	if !strings.Contains(view, "Page 1/5") {
		t.Error("expected view to show page 1/5")
	}
}

func TestDashboardViewShowsPageNumber(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	view := m.View()
	if !strings.Contains(view, "Page 1/5") {
		t.Error("expected view to show page 1/5")
	}
}

func TestDashboardViewShowsOverview(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	view := m.View()
	if !strings.Contains(view, "Pipeline overview") {
		t.Error("expected view to show Pipeline overview")
	}
}

func TestDashboardViewShowsEmptyState(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	view := m.View()
	if !strings.Contains(view, "no issues found") {
		t.Error("expected view to show 'no issues found' when all counts are zero")
	}
}

func TestDashboardNavigateNext(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	msg := tea.KeyMsg{Type: tea.KeyTab}

	result, _ := m.Update(msg)
	m2 := result.(dashboard.Model)
	view := m2.View()
	if !strings.Contains(view, "Page 2/5") {
		t.Errorf("expected page 2/5 after tab, got:\n%s", view)
	}
}

func TestDashboardNavigateNextKey(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}

	result, _ := m.Update(msg)
	m2 := result.(dashboard.Model)
	view := m2.View()
	if !strings.Contains(view, "Page 2/5") {
		t.Errorf("expected page 2/5 after 'n', got:\n%s", view)
	}
}

func TestDashboardNavigateBack(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	var r tea.Model
	r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model) // move to page 2
	r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model) // move to page 3

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}
	result, _ := m.Update(msg)
	m2 := result.(dashboard.Model)
	view := m2.View()
	if !strings.Contains(view, "Page 2/5") {
		t.Errorf("expected page 2/5 after 'p', got:\n%s", view)
	}
}

func TestDashboardNavigateBackShiftTab(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	var r tea.Model
	r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model) // move to page 2

	msg := tea.KeyMsg{Type: tea.KeyShiftTab}
	result, _ := m.Update(msg)
	m2 := result.(dashboard.Model)
	view := m2.View()
	if !strings.Contains(view, "Page 1/5") {
		t.Errorf("expected page 1/5 after shift+tab, got:\n%s", view)
	}
}

func TestDashboardNextDoesNotWrap(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	// Navigate to page 5 (quarantine) by pressing tab 4 times
	for i := 0; i < 4; i++ {
		var r tea.Model
		r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = r.(dashboard.Model)
	}
	// Should now be on page 5
	view := m.View()
	if !strings.Contains(view, "Page 5/5") {
		t.Errorf("expected page 5/5 after 4 tabs, got:\n%s", view)
	}

	// Try to navigate past the last page
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := result.(dashboard.Model)
	view2 := m2.View()
	if !strings.Contains(view2, "Page 5/5") {
		t.Errorf("expected to stay on page 5/5, got:\n%s", view2)
	}
}

func TestDashboardBackDoesNotWrap(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	msg := tea.KeyMsg{Type: tea.KeyShiftTab}

	result, _ := m.Update(msg)
	m2 := result.(dashboard.Model)
	view := m2.View()
	if !strings.Contains(view, "Page 1/5") {
		t.Errorf("expected to stay on page 1/5, got:\n%s", view)
	}
}

func TestDashboardNavigateRight(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	msg := tea.KeyMsg{Type: tea.KeyRight}

	result, _ := m.Update(msg)
	m2 := result.(dashboard.Model)
	view := m2.View()
	if !strings.Contains(view, "Page 2/5") {
		t.Errorf("expected page 2/5 after right arrow, got:\n%s", view)
	}
}

func TestDashboardNavigateLeft(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	var r tea.Model
	r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model) // page 2
	r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model) // page 3

	msg := tea.KeyMsg{Type: tea.KeyLeft}
	result, _ := m.Update(msg)
	m2 := result.(dashboard.Model)
	view := m2.View()
	if !strings.Contains(view, "Page 2/5") {
		t.Errorf("expected page 2/5 after left arrow, got:\n%s", view)
	}
}

func TestDashboardNavH(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	var r tea.Model
	r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model) // page 2
	r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model) // page 3

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	result, _ := m.Update(msg)
	m2 := result.(dashboard.Model)
	view := m2.View()
	if !strings.Contains(view, "Page 2/5") {
		t.Errorf("expected page 2/5 after 'h', got:\n%s", view)
	}
}

func TestDashboardNavL(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}

	result, _ := m.Update(msg)
	m2 := result.(dashboard.Model)
	view := m2.View()
	if !strings.Contains(view, "Page 2/5") {
		t.Errorf("expected page 2/5 after 'l', got:\n%s", view)
	}
}

func TestDashboardViewShowsIssueCounts(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	for _, sub := range []string{"test-ready", "done", ".quarantine"} {
		os.MkdirAll(filepath.Join(issuesDir, sub), 0755)
	}
	os.MkdirAll(issuesDir, 0755)

	os.WriteFile(filepath.Join(issuesDir, "a.md"), []byte("# Todo A\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, "test-ready", "b.md"), []byte("# Ready B\n"), 0644)

	m := dashboard.NewModel(config.Config{IssueDir: issuesDir})
	view := m.View()
	if !strings.Contains(view, "1 todo") {
		t.Error("expected view to show 1 todo")
	}
	if !strings.Contains(view, "1 test-ready") {
		t.Error("expected view to show 1 test-ready")
	}
}

func TestDashboardViewShowsIssueList(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(filepath.Join(issuesDir, "test-ready"), 0755)

	os.WriteFile(filepath.Join(issuesDir, "test-ready", "a.md"), []byte("# Issue A\n\nGitHub: #42\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, "test-ready", "b.md"), []byte("# Issue B\n"), 0644)

	m := dashboard.NewModel(config.Config{IssueDir: issuesDir})

	// Navigate to test-ready page (page 3: overview=1, todo=2, test-ready=3)
	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model) // todo
	r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model) // test-ready
	view := m.View()

	if !strings.Contains(view, "[test-ready] Issue A (#42)") {
		t.Errorf("expected '[test-ready] Issue A (#42)' in view, got:\n%s", view)
	}
	if !strings.Contains(view, "[test-ready] Issue B") {
		t.Errorf("expected '[test-ready] Issue B' in view, got:\n%s", view)
	}
}

func TestDashboardViewShowsGitHubNumber(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(filepath.Join(issuesDir, "test-ready"), 0755)

	os.WriteFile(filepath.Join(issuesDir, "test-ready", "gh.md"), []byte("# GH Issue\n\nGitHub: #42\n"), 0644)

	m := dashboard.NewModel(config.Config{IssueDir: issuesDir})
	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model)
	r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model)
	view := m.View()

	if !strings.Contains(view, "(#42)") {
		t.Errorf("expected GitHub number in view, got:\n%s", view)
	}
}

func TestDashboardViewHidesGitHubNumberWhenZero(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(filepath.Join(issuesDir, "todo"), 0755)

	os.WriteFile(filepath.Join(issuesDir, "plain.md"), []byte("# Plain Issue\n"), 0644)

	m := dashboard.NewModel(config.Config{IssueDir: issuesDir})
	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model)
	view := m.View()

	if strings.Contains(view, "(#0)") {
		t.Error("expected no '(#0)' suffix when GitHubNum is 0")
	}
}

func TestDashboardIssuePageShowsEmpty(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(filepath.Join(issuesDir, "done"), 0755)

	m := dashboard.NewModel(config.Config{IssueDir: issuesDir})
	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model)
	r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model)
	r, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = r.(dashboard.Model)
	view := m.View()

	if !strings.Contains(view, "(empty)") {
		t.Errorf("expected '(empty)' for done page with no issues, got:\n%s", view)
	}
}

func TestDashboardLoadsIssues(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	for _, sub := range []string{"test-ready", "done", ".quarantine"} {
		os.MkdirAll(filepath.Join(issuesDir, sub), 0755)
	}

	os.WriteFile(filepath.Join(issuesDir, "todo.md"), []byte("# Todo Item\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, "test-ready", "a.md"), []byte("# Ready A\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, "done", "b.md"), []byte("# Done B\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, ".quarantine", "c.md"), []byte("# Quar C\n"), 0644)

	m := dashboard.NewModel(config.Config{IssueDir: issuesDir})
	view := m.View()

	if !strings.Contains(view, "1 todo") {
		t.Errorf("expected 1 todo in view, got:\n%s", view)
	}
	if !strings.Contains(view, "1 test-ready") {
		t.Errorf("expected 1 test-ready in view, got:\n%s", view)
	}
	if !strings.Contains(view, "1 done") {
		t.Errorf("expected 1 done in view, got:\n%s", view)
	}
	if !strings.Contains(view, "quarantined: 1") {
		t.Errorf("expected quarantined: 1 in view, got:\n%s", view)
	}
}

func TestDashboardEmptySubdirs(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	for _, sub := range []string{"test-ready", "done", ".quarantine"} {
		os.MkdirAll(filepath.Join(issuesDir, sub), 0755)
	}

	m := dashboard.NewModel(config.Config{IssueDir: issuesDir})
	view := m.View()
	if !strings.Contains(view, "no issues found") {
		t.Errorf("expected 'no issues found', got:\n%s", view)
	}
}

func TestDashboardShowsUnparseableWarning(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(filepath.Join(issuesDir, "test-ready"), 0755)
	os.WriteFile(filepath.Join(issuesDir, "test-ready", "empty.md"), []byte{}, 0644)
	os.WriteFile(filepath.Join(issuesDir, "test-ready", "good.md"), []byte("# Good\n\nBody"), 0644)

	m := dashboard.NewModel(config.Config{IssueDir: issuesDir})
	view := m.View()
	if !strings.Contains(view, "empty.md (unparseable)") {
		t.Errorf("expected unparseable filename in view, got:\n%s", view)
	}
}

func TestDashboardNoUnparseableWarning(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(filepath.Join(issuesDir, "test-ready"), 0755)
	os.WriteFile(filepath.Join(issuesDir, "test-ready", "good.md"), []byte("# Good\n\nBody"), 0644)

	m := dashboard.NewModel(config.Config{IssueDir: issuesDir})
	view := m.View()
	if strings.Contains(view, "(unparseable)") {
		t.Errorf("unexpected unparseable marker in view:\n%s", view)
	}
}

func TestDashboardUpdateNonExistentKeyDoesNotChangeState(t *testing.T) {
	m := dashboard.NewModel(config.Config{})
	viewBefore := m.View()

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	result, _ := m.Update(msg)
	m2 := result.(dashboard.Model)
	viewAfter := m2.View()

	if m2.View() == "" {
		t.Error("expected non-empty view after unbound key")
	}
	if viewBefore != viewAfter {
		t.Error("expected view to remain unchanged for unbound key")
	}
}

func TestDashboardShowsRepoInfo(t *testing.T) {
	m := dashboard.NewModel(config.Config{IssueDir: t.TempDir(), Repo: "my-org/my-repo"})
	view := m.View()
	if !strings.Contains(view, "my-org/my-repo") {
		t.Errorf("expected repo info in view, got:\n%s", view)
	}
}

func TestRunRunOutput(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: filepath.Join(dir, "docs/issues")}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	runRun(3)

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "no issues found in pipeline") {
		t.Errorf("expected 'no issues found' message, got %q", got)
	}
	if exited {
		t.Error("expected no osExit call when there are no issues")
	}
}

func TestGitIgnorePatterns(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	gitignore := strings.TrimSuffix(filename, "cmd/loop/main_test.go") + ".gitignore"
	if _, err := os.Stat(gitignore); os.IsNotExist(err) {
		t.Skip(".gitignore not found")
	}
	data, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatalf("could not read .gitignore: %v", err)
	}
	content := string(data)

	patterns := []string{".loop/", "/loop", "*.tar.gz", "/vendor/", ".scratch/"}
	for _, p := range patterns {
		if !strings.Contains(content, p) {
			t.Errorf(".gitignore missing pattern: %s", p)
		}
	}
}

func TestRunStatusNoConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
	}
	defer func() { osExit = origExit }()

	runStatus()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "Not configured") {
		t.Errorf("expected 'Not configured' message, got %q", got)
	}
	if !exited {
		t.Error("expected osExit to be called")
	}
}

func TestRunStatusModelInit(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(filepath.Join(issuesDir, "test-ready"), 0755)
	os.MkdirAll(filepath.Join(issuesDir, "done"), 0755)
	os.WriteFile(filepath.Join(issuesDir, "test-ready", "a.md"), []byte("# Implement auth\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, "done", "b.md"), []byte("# Add logging\n"), 0644)

	cfg := config.Config{IssueDir: issuesDir, Repo: "my-org/my-repo"}
	m := status.NewModel(cfg)
	view := m.View()

	if !strings.Contains(view, "Pipeline overview") {
		t.Error("expected Pipeline overview in view")
	}
	if !strings.Contains(view, "1 test-ready") {
		t.Error("expected test-ready count 1 in view")
	}
	if !strings.Contains(view, "1 done") {
		t.Error("expected done count 1 in view")
	}
	if !strings.Contains(view, "my-org/my-repo") {
		t.Error("expected repo in view")
	}
}

func TestRunStatusModelCounts(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(filepath.Join(issuesDir, "test-ready"), 0755)
	os.MkdirAll(filepath.Join(issuesDir, "done"), 0755)
	os.MkdirAll(filepath.Join(issuesDir, ".quarantine"), 0755)
	os.WriteFile(filepath.Join(issuesDir, "a.md"), []byte("# Todo Item\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, "test-ready", "b.md"), []byte("# Ready A\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, "test-ready", "c.md"), []byte("# Ready B\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, "done", "d.md"), []byte("# Done C\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, ".quarantine", "e.md"), []byte("# Quar D\n"), 0644)

	cfg := config.Config{IssueDir: issuesDir, Repo: "test/repo"}
	m := status.NewModel(cfg)
	view := m.View()

	if !strings.Contains(view, "test/repo") {
		t.Error("expected repo info in view")
	}
}

func TestRunStatusModelEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := status.NewModel(cfg)
	view := m.View()

	if !strings.Contains(view, "no issues found") {
		t.Error("expected 'no issues found' in view")
	}
	if strings.Contains(view, "repo:") {
		t.Error("expected no repo line when repo is empty")
	}
}

func TestRunStatusModelNoIssues(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := status.NewModel(cfg)
	view := m.View()

	if !strings.Contains(view, "no issues found") {
		t.Error("expected 'no issues found' message")
	}
}

func TestRequireConfigWithConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: "docs/issues"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	requireConfig()
	if exited {
		t.Error("requireConfig should not exit when config exists")
	}
}

func TestRequireConfigWithError(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfgPath, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Dir(cfgPath), 0755)
	os.WriteFile(cfgPath, []byte("{invalid json}"), 0644)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	origSetup := runSetupFn
	runSetupFn = func() {}
	defer func() { runSetupFn = origSetup }()

	requireConfig()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "Error reading config") {
		t.Errorf("expected 'Error reading config' message, got %q", got)
	}
	if !exited {
		t.Error("expected osExit to be called")
	}
}

func TestRequireConfigNoConfigSetupSucceeds(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	origSetup := runSetupFn
	runSetupFn = func() {
		cfg := config.Config{IssueDir: "docs/issues"}
		if err := config.Save(cfg); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { runSetupFn = origSetup }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	requireConfig()
	if exited {
		t.Error("requireConfig should not exit after setup creates config")
	}
}

func TestRequireConfigNoConfigSetupFails(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	origSetup := runSetupFn
	runSetupFn = func() {}
	defer func() { runSetupFn = origSetup }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
	}
	defer func() { osExit = origExit }()

	requireConfig()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "Not configured") {
		t.Errorf("expected 'Not configured' message, got %q", got)
	}
	if !exited {
		t.Error("expected osExit to be called")
	}
}

func TestGoVersion(t *testing.T) {
	v := runtime.Version()
	if !strings.HasPrefix(v, "go1.") {
		t.Fatalf("unexpected Go version format: %q", v)
	}
	parts := strings.Split(strings.TrimPrefix(v, "go1."), ".")
	major := 0
	if len(parts) > 0 {
		major, _ = strconv.Atoi(parts[0])
	}
	if major < 22 {
		t.Fatalf("Go version %s is below minimum required 1.22", v)
	}
}

func TestParseArgsWithTimeoutFlag(t *testing.T) {
	cmd, code := parseArgs([]string{"--timeout", "600", "run", "5"})
	if cmd != cmdRun {
		t.Errorf("expected cmdRun, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if cliTimeout != 600 {
		t.Errorf("expected cliTimeout 600, got %d", cliTimeout)
	}
	if runN != 5 {
		t.Errorf("expected runN=5, got %d", runN)
	}
}

func TestParseArgsWithTimeoutFlagZero(t *testing.T) {
	cmd, code := parseArgs([]string{"--timeout", "0", "run", "3"})
	if cmd != cmdRun {
		t.Errorf("expected cmdRun, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if cliTimeout != 0 {
		t.Errorf("expected cliTimeout 0, got %d", cliTimeout)
	}
}

func TestParseArgsRunWithRepairFlag(t *testing.T) {
	cmd, code := parseArgs([]string{"--repair", "run", "5"})
	if cmd != cmdRun {
		t.Errorf("expected cmdRun, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !cliRepair {
		t.Error("expected cliRepair to be true when --repair is set")
	}
	if runN != 5 {
		t.Errorf("expected runN=5, got %d", runN)
	}
}

func TestParseArgsRunRepairFalse(t *testing.T) {
	cmd, code := parseArgs([]string{"--repair=false", "run", "3"})
	if cmd != cmdRun {
		t.Errorf("expected cmdRun, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if cliRepair {
		t.Error("expected cliRepair to be false when --repair=false")
	}
}

func TestParseArgsWithTimeoutFlagNoRun(t *testing.T) {
	cmd, code := parseArgs([]string{"--timeout", "300", "status"})
	if cmd != cmdStatus {
		t.Errorf("expected cmdStatus, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunRunWithTimeout(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{
		IssueDir:     filepath.Join(dir, "docs/issues"),
		AgentTimeout: 120,
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	// Set CLI timeout to 0, should use config value
	cliTimeout = 0

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	runRun(3)

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "no issues found in pipeline") {
		t.Errorf("expected 'no issues found' message, got %q", got)
	}
	if exited {
		t.Error("expected no osExit call when there are no issues")
	}
}

func TestRunRunWithCLITimeout(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{
		IssueDir:     filepath.Join(dir, "docs/issues"),
		AgentTimeout: 120,
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	// Set CLI timeout, should override config value
	cliTimeout = 600

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	runRun(3)

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "no issues found in pipeline") {
		t.Errorf("expected 'no issues found' message, got %q", got)
	}
	if exited {
		t.Error("expected no osExit call when there are no issues")
	}
}

func TestParseArgsRunWithIssueNumber(t *testing.T) {
	cmd, code := parseArgs([]string{"run", "5", "42"})
	if cmd != cmdRun {
		t.Errorf("expected cmdRun, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if runN != 5 {
		t.Errorf("expected runN=5, got %d", runN)
	}
	if runIssueNum != 42 {
		t.Errorf("expected runIssueNum=42, got %d", runIssueNum)
	}
}

func TestParseArgsRunWithInvalidIssueNumber(t *testing.T) {
	cmd, code := parseArgs([]string{"run", "5", "extra"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsRunWithZeroIssueNumber(t *testing.T) {
	cmd, code := parseArgs([]string{"run", "5", "0"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsRunWithNegativeIssueNumber(t *testing.T) {
	cmd, code := parseArgs([]string{"run", "5", "-1"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsRunHeadlessFlag(t *testing.T) {
	cmd, code := parseArgs([]string{"--headless", "run", "5"})
	if cmd != cmdRun {
		t.Errorf("expected cmdRun, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if runN != 5 {
		t.Errorf("expected runN=5, got %d", runN)
	}
	if !cliHeadless {
		t.Error("expected cliHeadless to be true when --headless is set")
	}
}

func TestParseArgsRunHeadlessFalse(t *testing.T) {
	cliHeadless = false
	cmd, code := parseArgs([]string{"--headless=false", "run", "3"})
	if cmd != cmdRun {
		t.Errorf("expected cmdRun, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if cliHeadless {
		t.Error("expected cliHeadless to be false when --headless=false")
	}
}

func TestRunRunHeadlessNoIssues(t *testing.T) {
	cliHeadless = true
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: filepath.Join(dir, "docs/issues")}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	runRun(3)

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "no issues found in pipeline") {
		t.Errorf("expected 'no issues found' message, got %q", got)
	}
	if exited {
		t.Error("expected no osExit call when there are no issues")
	}
}

func TestMainCommandWithHeadless(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: "docs/issues"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	os.Args = []string{"loop", "--headless", "run", "5"}
	defer func() { os.Args = oldArgs }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	main()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "no issues found in pipeline") {
		t.Errorf("expected 'no issues found' message in headless output, got %q", got)
	}
	if exited {
		t.Error("expected no osExit call when there are no issues")
	}
}

func TestParseArgsVersionWithSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"--version", "setup"})
	if cmd != cmdVersion {
		t.Errorf("expected cmdVersion, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsHelpOverVersion(t *testing.T) {
	cmd, code := parseArgs([]string{"--help", "--version"})
	if cmd != cmdHelp {
		t.Errorf("expected cmdHelp, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsUnknownFlag(t *testing.T) {
	cmd, code := parseArgs([]string{"--unknown"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsRunFloatArg(t *testing.T) {
	cmd, code := parseArgs([]string{"run", "5.5"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsDoubleDash(t *testing.T) {
	cmd, code := parseArgs([]string{"--"})
	if cmd != cmdTUI {
		t.Errorf("expected cmdTUI (-- consumed, no args remain), got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsDoubleDashBeforeRun(t *testing.T) {
	cmd, code := parseArgs([]string{"--", "run", "5"})
	if cmd != cmdRun {
		t.Errorf("expected cmdRun, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsRunLargeNumber(t *testing.T) {
	cmd, code := parseArgs([]string{"run", "99999"})
	if cmd != cmdRun {
		t.Errorf("expected cmdRun, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if runN != 99999 {
		t.Errorf("expected runN=99999, got %d", runN)
	}
}

func TestParseArgsOnlyFlagLikeArg(t *testing.T) {
	cmd, code := parseArgs([]string{"--unknown-flag"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestPrintUsage(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	printUsage()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "Usage:") {
		t.Errorf("expected output to contain 'Usage:', got %q", got)
	}
	if !strings.Contains(got, "loop [command]") {
		t.Errorf("expected output to contain 'loop [command]', got %q", got)
	}
}

func TestOSArchDefaults(t *testing.T) {
	if GOOS != runtime.GOOS {
		t.Errorf("expected GOOS %q, got %q", runtime.GOOS, GOOS)
	}
	if GOARCH != runtime.GOARCH {
		t.Errorf("expected GOARCH %q, got %q", runtime.GOARCH, GOARCH)
	}
}

func TestVersionDefault(t *testing.T) {
	if Version != "dev" {
		t.Errorf("expected default Version to be \"dev\", got %q", Version)
	}
}

func TestMainCommandHelp(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"loop", "--help"}
	defer func() { os.Args = oldArgs }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	var exitCode int
	origExit := osExit
	osExit = func(code int) {
		exited = true
		exitCode = code
	}
	defer func() { osExit = origExit }()

	main()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Error("expected osExit to be called")
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(buf.String(), "Usage:") {
		t.Error("expected usage output on stderr")
	}
}

func TestMainCommandVersion(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"loop", "--version"}
	defer func() { os.Args = oldArgs }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	exited := false
	var exitCode int
	origExit := osExit
	osExit = func(code int) {
		exited = true
		exitCode = code
	}
	defer func() { osExit = origExit }()

	main()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Error("expected osExit to be called")
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	got := buf.String()
	if !strings.Contains(got, "loop v") {
		t.Errorf("expected version output, got %q", got)
	}
	if !strings.Contains(got, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Errorf("expected OS/arch in version output, got %q", got)
	}
}

func TestMainCommandSetup(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"loop", "setup"}
	defer func() { os.Args = oldArgs }()

	called := false
	origSetup := runSetupFn
	runSetupFn = func() { called = true }
	defer func() { runSetupFn = origSetup }()

	main()

	if !called {
		t.Error("expected runSetup to be called")
	}
}

func TestMainCommandTuiNoConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	oldArgs := os.Args
	os.Args = []string{"loop"}
	defer func() { os.Args = oldArgs }()

	setupCalled := false
	origSetup := runSetupFn
	runSetupFn = func() {
		setupCalled = true
		cfg := config.Config{IssueDir: "docs/issues"}
		if err := config.Save(cfg); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { runSetupFn = origSetup }()

	tuiCalled := false
	origTUI := runTUIFn
	runTUIFn = func(cfg config.Config) { tuiCalled = true }
	defer func() { runTUIFn = origTUI }()

	main()

	if !setupCalled {
		t.Error("expected setup to be called on first run")
	}
	if !tuiCalled {
		t.Error("expected TUI to be called after setup completes")
	}
}

func TestMainCommandTuiWithConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: "docs/issues"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	os.Args = []string{"loop"}
	defer func() { os.Args = oldArgs }()

	setupCalled := false
	origSetup := runSetupFn
	runSetupFn = func() { setupCalled = true }
	defer func() { runSetupFn = origSetup }()

	tuiCalled := false
	origTUI := runTUIFn
	runTUIFn = func(cfg config.Config) { tuiCalled = true }
	defer func() { runTUIFn = origTUI }()

	main()

	if setupCalled {
		t.Error("expected setup NOT to be called when config exists")
	}
	if !tuiCalled {
		t.Error("expected TUI to be called when config exists")
	}
}

func TestMainCommandStatus(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: "docs/issues"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	os.Args = []string{"loop", "status"}
	defer func() { os.Args = oldArgs }()

	called := false
	origStatus := runStatusFn
	runStatusFn = func() { called = true }
	defer func() { runStatusFn = origStatus }()

	main()

	if !called {
		t.Error("expected runStatusFn to be called for 'loop status'")
	}
}

func TestMainCommandUnknown(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"loop", "unknown"}
	defer func() { os.Args = oldArgs }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	var exitCode int
	origExit := osExit
	osExit = func(code int) {
		exited = true
		exitCode = code
	}
	defer func() { osExit = origExit }()

	main()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Error("expected osExit to be called")
	}
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(buf.String(), "Usage:") {
		t.Error("expected usage output on stderr")
	}
}

func TestPrintShutdownSummary(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	for _, sub := range []string{"test-ready", "done", ".quarantine"} {
		os.MkdirAll(filepath.Join(issuesDir, sub), 0755)
	}
	os.MkdirAll(issuesDir, 0755)

	os.WriteFile(filepath.Join(issuesDir, "todo.md"), []byte("# Todo\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, "test-ready", "a.md"), []byte("# Ready\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, "done", "b.md"), []byte("# Done\n"), 0644)
	os.WriteFile(filepath.Join(issuesDir, ".quarantine", "c.md"), []byte("# Quar\n"), 0644)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	printShutdownSummary(3, 5, issuesDir)

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	if !strings.Contains(got, "Run summary") {
		t.Errorf("expected 'Run summary' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "3/5") {
		t.Errorf("expected '3/5' iterations in output, got:\n%s", got)
	}
	if !strings.Contains(got, "todo: 1") {
		t.Errorf("expected 'todo: 1' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "test-ready: 1") {
		t.Errorf("expected 'test-ready: 1' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "done: 1") {
		t.Errorf("expected 'done: 1' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "quarantined: 1") {
		t.Errorf("expected 'quarantined: 1' in output, got:\n%s", got)
	}
}

func TestPrintShutdownSummaryEmptyDir(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	printShutdownSummary(2, 3, issuesDir)

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	if !strings.Contains(got, "2/3") {
		t.Errorf("expected '2/3' iterations, got:\n%s", got)
	}
	if !strings.Contains(got, "Run summary") {
		t.Errorf("expected 'Run summary' in output, got:\n%s", got)
	}
}

func TestParseArgsCheckSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"check"})
	if cmd != cmdCheck {
		t.Errorf("expected cmdCheck, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunCheckNoConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	var exitCode int
	origExit := osExit
	osExit = func(code int) {
		exited = true
		exitCode = code
	}
	defer func() { osExit = origExit }()

	runCheck()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Error("expected osExit to be called")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(buf.String(), "Not configured") {
		t.Errorf("expected 'Not configured' message, got %q", buf.String())
	}
}

func TestRunCheckValid(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	issuesDir := filepath.Join(dir, "docs/issues")
	cfg := config.Config{IssueDir: issuesDir}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	issue.Create(issuesDir, issue.StateTodo, "Issue One", "GitHub: #1\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")
	issue.Create(issuesDir, issue.StateDone, "Issue Two", "GitHub: #2\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runCheck()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if exited {
		t.Error("expected no osExit call when pipeline is valid")
	}
	if !strings.Contains(buf.String(), "No issues found") {
		t.Errorf("expected 'No issues found' message, got %q", buf.String())
	}
}

func TestRunCheckWithIssues(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	issuesDir := filepath.Join(dir, "docs/issues")
	cfg := config.Config{IssueDir: issuesDir}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	issue.Create(issuesDir, issue.StateTodo, "Issue One", "GitHub: #1\n")
	issue.Create(issuesDir, issue.StateTestReady, "Issue One Dup", "GitHub: #1\n")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	var exitCode int
	origExit := osExit
	osExit = func(code int) {
		exited = true
		exitCode = code
	}
	defer func() { osExit = origExit }()

	runCheck()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Error("expected osExit to be called for issues")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(buf.String(), "GitHub issue #1 appears in multiple files") {
		t.Errorf("expected duplicate GitHub issue message, got %q", buf.String())
	}
}

func TestParseArgsRestoreSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"restore"})
	if cmd != cmdRestore {
		t.Errorf("expected cmdRestore, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestMainCommandRestore(t *testing.T) {
	dir := t.TempDir()
	testhelper.InitRepo(t, dir)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	oldArgs := os.Args
	os.Args = []string{"loop", "restore"}
	defer func() { os.Args = oldArgs }()

	called := false
	origRestore := runRestoreFn
	runRestoreFn = func() { called = true }
	defer func() { runRestoreFn = origRestore }()

	main()

	if !called {
		t.Error("expected runRestoreFn to be called for 'loop restore'")
	}
}

func TestRunRestoreNoContext(t *testing.T) {
	dir := t.TempDir()
	testhelper.InitRepo(t, dir)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	runRestore()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "no git context to restore") {
		t.Errorf("expected 'no git context to restore', got %q", got)
	}
	if exited {
		t.Error("expected no osExit call when there is no context to restore")
	}
}

func TestParseArgsChecksumVerifySubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"checksum", "verify"})
	if cmd != cmdChecksum {
		t.Errorf("expected cmdChecksum, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsChecksumInvalidSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"checksum", "add"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsChecksumNoSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"checksum"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestRunChecksumVerifyNoConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	var exitCode int
	origExit := osExit
	osExit = func(code int) {
		exited = true
		exitCode = code
	}
	defer func() { osExit = origExit }()

	runChecksumVerify()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Error("expected osExit to be called")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(buf.String(), "Not configured") {
		t.Errorf("expected 'Not configured' message, got %q", buf.String())
	}
}

func TestRunChecksumVerifyValid(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	issuesDir := filepath.Join(dir, "docs/issues")
	cfg := config.Config{IssueDir: issuesDir}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	os.MkdirAll(issuesDir, 0755)
	os.WriteFile(filepath.Join(issuesDir, "a.md"), []byte("# A\n\nBody\n"), 0644)
	if _, err := issue.AddMissingChecksums(issuesDir, true); err != nil {
		t.Fatal(err)
	}

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runChecksumVerify()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if exited {
		t.Error("expected no osExit call when checksums are valid")
	}
	if !strings.Contains(buf.String(), "ok:") {
		t.Errorf("expected 'ok:' in output, got %q", buf.String())
	}
}

func TestRunChecksumVerifyWithFailures(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	issuesDir := filepath.Join(dir, "docs/issues")
	cfg := config.Config{IssueDir: issuesDir}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	os.MkdirAll(issuesDir, 0755)
	path := filepath.Join(issuesDir, "a.md")
	os.WriteFile(path, []byte("# A\n\nBody\n"), 0644)
	if _, err := issue.AddMissingChecksums(issuesDir, true); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	tampered := strings.ReplaceAll(string(data), "Body", "Tampered")
	os.WriteFile(path, []byte(tampered), 0644)

	exited := false
	var exitCode int
	origExit := osExit
	osExit = func(code int) {
		exited = true
		exitCode = code
	}
	defer func() { osExit = origExit }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	runChecksumVerify()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Error("expected osExit to be called for checksum failures")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(buf.String(), "FAIL") {
		t.Errorf("expected 'FAIL' in output, got %q", buf.String())
	}
}

func TestMainCommandChecksum(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: "docs/issues"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	os.Args = []string{"loop", "checksum", "verify"}
	defer func() { os.Args = oldArgs }()

	called := false
	origChecksum := runChecksumVerifyFn
	runChecksumVerifyFn = func() { called = true }
	defer func() { runChecksumVerifyFn = origChecksum }()

	main()

	if !called {
		t.Error("expected runChecksumVerifyFn to be called for 'loop checksum verify'")
	}
}

func TestMainCommandChecksumUnknown(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: "docs/issues"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	os.Args = []string{"loop", "checksum", "unknown"}
	defer func() { os.Args = oldArgs }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	main()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Error("expected osExit to be called for unknown checksum subcommand")
	}
}

func TestMainCommandChecksumNoSubcommand(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: "docs/issues"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	os.Args = []string{"loop", "checksum"}
	defer func() { os.Args = oldArgs }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	main()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Error("expected osExit to be called for checksum with no subcommand")
	}
}

func TestMainCommandCheck(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: "docs/issues"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	os.Args = []string{"loop", "check"}
	defer func() { os.Args = oldArgs }()

	called := false
	origCheck := runCheckFn
	runCheckFn = func() { called = true }
	defer func() { runCheckFn = origCheck }()

	main()

	if !called {
		t.Error("expected runCheckFn to be called for 'loop check'")
	}
}

func TestRunCheckReopensClosedIssues(t *testing.T) {
	github.ResetAuthCheck()

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	issuesDir := filepath.Join(dir, "docs/issues")
	cfg := config.Config{IssueDir: issuesDir, Repo: "owner/repo"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	issue.Create(issuesDir, issue.StateTodo, "Pending Issue", "GitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")

	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "state" ] && [ "$8" = "--jq" ] && [ "$9" = ".state" ]; then
	echo "CLOSED"
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "reopen" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "comment" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--body" ]; then
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	runCheck()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	if !strings.Contains(got, "reopened prematurely closed GitHub issue #42") {
		t.Errorf("expected reopen message for issue #42, got:\n%s", got)
	}
	if exited {
		t.Error("expected no osExit call after reopening")
	}
}

func TestParseArgsDownloadSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"download"})
	if cmd != cmdDownload {
		t.Errorf("expected cmdDownload, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestMainCommandDownload(t *testing.T) {
	dir := t.TempDir()
	testhelper.InitRepo(t, dir)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: filepath.Join(dir, "docs/issues"), Repo: "owner/repo"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	os.Args = []string{"loop", "download"}
	defer func() { os.Args = oldArgs }()

	called := false
	origDownload := runDownloadFn
	runDownloadFn = func() { called = true }
	defer func() { runDownloadFn = origDownload }()

	main()

	if !called {
		t.Error("expected runDownloadFn to be called for 'loop download'")
	}
}

func TestMainCommandDownloadNoConfig(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	exited := false
	origExit := osExit
	osExit = func(code int) { exited = true }
	defer func() { osExit = origExit }()

	runDownload()

	if !exited {
		t.Error("expected osExit for missing config")
	}
}

func TestMainCommandDownloadNoRepo(t *testing.T) {
	dir := t.TempDir()
	testhelper.InitRepo(t, dir)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: filepath.Join(dir, "docs/issues"), Repo: ""}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	// Prevent osExit from killing the test process.
	origExit := osExit
	osExit = func(code int) {}
	defer func() { osExit = origExit }()

	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	runDownload()

	w.Close()
	var buf strings.Builder
	io.Copy(&buf, r)
	output := buf.String()

	// Should NOT mention repo config — that check was removed.
	if strings.Contains(output, "No GitHub repo configured") {
		t.Error("unexpected 'No GitHub repo configured' error — download no longer requires a repo")
	}
	// Should at least attempt the download.
	if !strings.Contains(output, "Downloading latest loop release") {
		t.Errorf("expected download attempt, got:\n%s", output)
	}
}

func TestParseArgsCommandsSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"commands"})
	if cmd != cmdCommands {
		t.Errorf("expected cmdCommands, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestMainCommandCommands(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"loop", "commands"}
	defer func() { os.Args = oldArgs }()

	called := false
	origCommands := runCommandsFn
	runCommandsFn = func() { called = true }
	defer func() { runCommandsFn = origCommands }()

	main()

	if !called {
		t.Error("expected runCommandsFn to be called for 'loop commands'")
	}
}

func TestRunCommandsOutput(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runCommands()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	if !strings.Contains(got, "Commands:") {
		t.Errorf("expected 'Commands:' header in output, got:\n%s", got)
	}
	if !strings.Contains(got, "loop setup") {
		t.Errorf("expected 'loop setup' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "loop commands") {
		t.Errorf("expected 'loop commands' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Flags:") {
		t.Errorf("expected 'Flags:' section in output, got:\n%s", got)
	}
}

func TestParseArgsRepairSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"repair"})
	if cmd != cmdRepair {
		t.Errorf("expected cmdRepair, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestRunRepairNoConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	runRepair()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Error("expected osExit to be called")
	}
	if !strings.Contains(buf.String(), "Not configured") {
		t.Errorf("expected 'Not configured' message, got %q", buf.String())
	}
}

func TestRunRepairValid(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	issuesDir := filepath.Join(dir, "docs/issues")
	cfg := config.Config{IssueDir: issuesDir}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	issue.Create(issuesDir, issue.StateTodo, "Issue One", "GitHub: #1\nStatus: ready-for-agent\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runRepair()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if exited {
		t.Error("expected no osExit call when pipeline is valid")
	}
	if !strings.Contains(buf.String(), "No issues found") {
		t.Errorf("expected 'No issues found' message, got %q", buf.String())
	}
}

func TestRunRepairStripsAndPromotes(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	issuesDir := filepath.Join(dir, "docs/issues")
	cfg := config.Config{IssueDir: issuesDir}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	readyDir := filepath.Join(issuesDir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	// File with filled UAT Results (should be promoted)
	filled := "# 01 - Tested\n\n## What to build\n\nStuff\n\n## UAT Results\n\n| Step | Result |\n| --- | --- |\n| Check | Pass |\n"
	os.WriteFile(filepath.Join(readyDir, "filled.md"), []byte(filled), 0644)

	// File with empty UAT Results (should be stripped)
	empty := "# 02 - Empty\n\n## What to build\n\nStuff\n\n## UAT Results\n"
	os.WriteFile(filepath.Join(readyDir, "empty.md"), []byte(empty), 0644)

	exited := false
	origExit := osExit
	osExit = func(code int) {
		exited = true
	}
	defer func() { osExit = origExit }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	runRepair()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if exited {
		t.Error("expected no osExit call after repair")
	}
	got := buf.String()
	if !strings.Contains(got, "stripped empty UAT Results from 1 file(s)") {
		t.Errorf("expected strip message, got:\n%s", got)
	}
	if !strings.Contains(got, "found 1 test-ready file(s) with populated UAT Results") {
		t.Errorf("expected stuck message, got:\n%s", got)
	}
}

func TestParseArgsScreenshot(t *testing.T) {
	cmd, code := parseArgs([]string{"screenshot"})
	if cmd != cmdScreenshot {
		t.Errorf("expected cmdScreenshot, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestScreenshotOutputToFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	issuesDir := filepath.Join(dir, "docs/issues")
	cfg := config.Config{IssueDir: issuesDir}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	issue.Create(issuesDir, issue.StateTodo, "Test Issue", "## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")
	issue.Create(issuesDir, issue.StateDone, "Done Issue", "## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	runScreenshot()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "screenshot saved") {
		t.Errorf("expected 'screenshot saved' message, got %q", got)
	}
	if !strings.Contains(got, ".txt") {
		t.Errorf("expected .txt file in message, got %q", got)
	}

	name := strings.TrimSpace(strings.TrimPrefix(got, "screenshot saved: "))
	if name != "" {
		if _, err := os.Stat(filepath.Join(".", name)); os.IsNotExist(err) {
			t.Errorf("expected screenshot file %s to exist", name)
		}
	}
}

func TestMainCommandScreenshot(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	cfg := config.Config{IssueDir: "docs/issues"}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	os.Args = []string{"loop", "screenshot"}
	defer func() { os.Args = oldArgs }()

	called := false
	origScreenshot := runScreenshotFn
	runScreenshotFn = func() { called = true }
	defer func() { runScreenshotFn = origScreenshot }()

	main()

	if !called {
		t.Error("expected runScreenshotFn to be called for 'loop screenshot'")
	}
}

func TestParseArgsCompletionSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"completion"})
	if cmd != cmdCompletion {
		t.Errorf("expected cmdCompletion, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsCompletionBashSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"completion", "bash"})
	if cmd != cmdCompletion {
		t.Errorf("expected cmdCompletion, got %d", cmd)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestParseArgsCompletionInvalidSubcommand(t *testing.T) {
	cmd, code := parseArgs([]string{"completion", "zsh"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestParseArgsCompletionTooManyArgs(t *testing.T) {
	cmd, code := parseArgs([]string{"completion", "bash", "extra"})
	if cmd != cmdUnknown {
		t.Errorf("expected cmdUnknown, got %d", cmd)
	}
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestPrintBashCompletion(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	printBashCompletion()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	if !strings.Contains(got, "_loop_completions") {
		t.Errorf("expected '_loop_completions' function, got:\n%s", got)
	}
	if !strings.Contains(got, "complete -F _loop_completions loop") {
		t.Errorf("expected 'complete -F' line, got:\n%s", got)
	}
	if !strings.Contains(got, "setup") {
		t.Errorf("expected 'setup' in completion output, got:\n%s", got)
	}
}

func TestMainCommandCompletion(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"loop", "completion", "bash"}
	defer func() { os.Args = oldArgs }()

	called := false
	origCompletion := runCompletionFn
	runCompletionFn = func() { called = true }
	defer func() { runCompletionFn = origCompletion }()

	main()

	if !called {
		t.Error("expected runCompletionFn to be called for 'loop completion bash'")
	}
}

func TestMainCommandCompletionBare(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"loop", "completion"}
	defer func() { os.Args = oldArgs }()

	called := false
	origCompletion := runCompletionFn
	runCompletionFn = func() { called = true }
	defer func() { runCompletionFn = origCompletion }()

	main()

	if !called {
		t.Error("expected runCompletionFn to be called for 'loop completion'")
	}
}
