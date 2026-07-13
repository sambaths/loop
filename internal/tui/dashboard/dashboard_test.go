package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sambaths/loop/internal/config"
	"github.com/sambaths/loop/internal/issue"
)

func setupIssueDirs(t *testing.T, dir string) {
	t.Helper()
	for _, sub := range []string{"test-ready", "done", ".quarantine"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func writeIssue(t *testing.T, path, title string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("# "+title+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNewModel(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	if m.page != pageOverview {
		t.Errorf("expected pageOverview, got %d", m.page)
	}
}

func TestNewModelLoadsIssues(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "a.md"), "Implement auth")
	writeIssue(t, filepath.Join(issuesDir, "done", "b.md"), "Add logging")
	writeIssue(t, filepath.Join(issuesDir, ".quarantine", "c.md"), "Flaky test")

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	if len(m.testReady) != 1 {
		t.Errorf("expected 1 test-ready issue, got %d", len(m.testReady))
	}
	if len(m.done) != 1 {
		t.Errorf("expected 1 done issue, got %d", len(m.done))
	}
	if len(m.quarantined) != 1 {
		t.Errorf("expected 1 quarantined issue, got %d", len(m.quarantined))
	}
	if len(m.todo) != 0 {
		t.Errorf("expected 0 todo issues, got %d", len(m.todo))
	}
}

func TestNewModelEmptySubdirsCountsZero(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	if len(m.todo) != 0 {
		t.Errorf("expected 0 todo issues, got %d", len(m.todo))
	}
	if len(m.testReady) != 0 {
		t.Errorf("expected 0 test-ready issues, got %d", len(m.testReady))
	}
	if len(m.done) != 0 {
		t.Errorf("expected 0 done issues, got %d", len(m.done))
	}
	if len(m.quarantined) != 0 {
		t.Errorf("expected 0 quarantined issues, got %d", len(m.quarantined))
	}
	if len(m.warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(m.warnings))
	}
}

func TestInitReturnsNil(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	cmd := m.Init()
	if cmd != nil {
		t.Error("expected nil cmd from Init")
	}
}

func TestQuit(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, cmd := m.Update(msg)
	m2 := result.(Model)

	if !m2.quit {
		t.Error("expected quit to be true")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestCtrlC(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, cmd := m.Update(msg)
	m2 := result.(Model)

	if !m2.quit {
		t.Error("expected quit to be true")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestNavigateNext(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	m.page = pageOverview
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := result.(Model)
	if m2.page != pageTodo {
		t.Errorf("tab: expected pageTodo, got %d", m2.page)
	}

	m.page = pageOverview
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m2 = result.(Model)
	if m2.page != pageTodo {
		t.Errorf("n: expected pageTodo, got %d", m2.page)
	}

	m.page = pageOverview
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m2 = result.(Model)
	if m2.page != pageTodo {
		t.Errorf("l: expected pageTodo, got %d", m2.page)
	}
}

func TestNavigateBack(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	m.page = pageTestReady
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m2 := result.(Model)
	if m2.page != pageTodo {
		t.Errorf("shift+tab: expected pageTodo, got %d", m2.page)
	}

	m.page = pageTestReady
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m2 = result.(Model)
	if m2.page != pageTodo {
		t.Errorf("p: expected pageTodo, got %d", m2.page)
	}

	m.page = pageTestReady
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m2 = result.(Model)
	if m2.page != pageTodo {
		t.Errorf("h: expected pageTodo, got %d", m2.page)
	}
}

func TestNextStaysAtLastPage(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.page = pageQuarantine

	msg := tea.KeyMsg{Type: tea.KeyTab}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.page != pageQuarantine {
		t.Errorf("expected to stay on pageQuarantine, got %d", m2.page)
	}
}

func TestPrevStaysAtFirstPage(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	msg := tea.KeyMsg{Type: tea.KeyShiftTab}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.page != pageOverview {
		t.Errorf("expected to stay on pageOverview, got %d", m2.page)
	}
}

func TestViewOverviewEmpty(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	view := m.View()

	if !strings.Contains(view, "no issues found") {
		t.Error("expected overview to say 'no issues found'")
	}
	if !strings.Contains(view, "issues dir") {
		t.Error("expected overview to show issues dir")
	}
	if !strings.Contains(view, "loop v") {
		t.Error("expected view to contain version header")
	}
}

func TestViewOverviewWithCounts(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "a.md"), "Todo item")
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "b.md"), "Ready item")
	writeIssue(t, filepath.Join(issuesDir, "done", "c.md"), "Done item")
	writeIssue(t, filepath.Join(issuesDir, ".quarantine", "d.md"), "Quar item")

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	view := m.View()

	if !strings.Contains(view, "Pipeline overview") {
		t.Error("expected overview header")
	}
	if !strings.Contains(view, "todo") || !strings.Contains(view, "1 todo") {
		t.Error("expected todo count in overview")
	}
	if !strings.Contains(view, "test-ready") || !strings.Contains(view, "1 test-ready") {
		t.Error("expected test-ready count in overview")
	}
	if !strings.Contains(view, "done") || !strings.Contains(view, "1 done") {
		t.Error("expected done count in overview")
	}
	if !strings.Contains(view, "quarantined: 1") {
		t.Error("expected quarantined count in overview")
	}
	if !strings.Contains(view, "%") {
		t.Error("expected progress percentage in overview")
	}
}

func TestViewOverviewWithRepo(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "a.md"), "Issue")

	cfg := config.Config{IssueDir: issuesDir, Repo: "my-org/my-repo"}
	m := NewModel(cfg)
	view := m.View()

	if !strings.Contains(view, "my-org/my-repo") {
		t.Error("expected repo in overview view")
	}
}

func TestViewShowsIssueTitles(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "a.md"), "Implement auth")
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "b.md"), "Add database")

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.page = pageTestReady

	view := m.View()
	if !strings.Contains(view, "[test-ready] Implement auth") {
		t.Error("expected '[test-ready] Implement auth' in test-ready view")
	}
	if !strings.Contains(view, "[test-ready] Add database") {
		t.Error("expected '[test-ready] Add database' in test-ready view")
	}
}

func TestViewPageEmpty(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.page = pageTestReady

	view := m.View()
	if !strings.Contains(view, "(empty)") {
		t.Error("expected '(empty)' for page with no issues")
	}
}

func TestViewShowsNavigation(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	view := m.View()

	if !strings.Contains(view, "Page 1/5") {
		t.Error("expected 'Page 1/5' navigation in view")
	}
}

func TestViewShowsPageNumber(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.page = pageTestReady

	view := m.View()
	if !strings.Contains(view, "Page 3/5") {
		t.Errorf("expected 'Page 3/5' for test-ready page, got view:\n%s", view)
	}
}

func TestViewAfterQuit(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.quit = true

	view := m.View()
	if view != "" {
		t.Error("expected empty view after quit")
	}
}

func TestViewShowsGitHubNumber(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	path := filepath.Join(issuesDir, "test-ready", "gh-issue.md")
	content := "# GitHub Issue\n\nGitHub: #42\n"
	os.WriteFile(path, []byte(content), 0644)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.page = pageTestReady

	view := m.View()
	if !strings.Contains(view, "[test-ready] GitHub Issue (#42)") {
		t.Errorf("expected '[test-ready] GitHub Issue (#42)' in view, got:\n%s", view)
	}
}

func TestViewHidesGitHubNumberWhenZero(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "a.md"), "Plain Issue")

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.page = pageTestReady

	view := m.View()
	if strings.Contains(view, "(#0)") {
		t.Error("expected no '(#0)' suffix when GitHubNum is 0")
	}
}

func TestHelpShown(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	view := m.View()

	if !strings.Contains(view, "q to quit") {
		t.Error("expected help text in view")
	}
	if !strings.Contains(view, "navigate") {
		t.Error("expected navigation help in view")
	}
}

func TestTodosFromRootDir(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "backlog.md"), "Backlog item")
	writeIssue(t, filepath.Join(issuesDir, "idea.md"), "Idea")

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.page = pageTodo

	if len(m.todo) != 2 {
		t.Errorf("expected 2 todo issues, got %d", len(m.todo))
	}

	view := m.View()
	if !strings.Contains(view, "[todo] Backlog item") {
		t.Error("expected '[todo] Backlog item' in todo view")
	}
	if !strings.Contains(view, "[todo] Idea") {
		t.Error("expected '[todo] Idea' in todo view")
	}
}

func TestPipelineBarEmpty(t *testing.T) {
	issuesDir := filepath.Join(t.TempDir(), "issues")
	os.MkdirAll(issuesDir, 0755)
	m := NewModel(config.Config{IssueDir: issuesDir})
	bar := m.pipelineBar()
	if !strings.Contains(bar, "no issues found") {
		t.Errorf("expected 'no issues found' for empty dir, got: %s", bar)
	}
}

func TestPipelineBarShowsCounts(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "todo.md"), "Todo item")
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "a.md"), "Ready")
	writeIssue(t, filepath.Join(issuesDir, "done", "b.md"), "Done")
	writeIssue(t, filepath.Join(issuesDir, ".quarantine", "c.md"), "Quar")

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	bar := m.pipelineBar()

	if !strings.Contains(bar, "1 todo") {
		t.Errorf("expected '1 todo' in bar, got: %s", bar)
	}
	if !strings.Contains(bar, "1 test-ready") {
		t.Errorf("expected '1 test-ready' in bar, got: %s", bar)
	}
	if !strings.Contains(bar, "1 done") {
		t.Errorf("expected '1 done' in bar, got: %s", bar)
	}
	if !strings.Contains(bar, "quarantined: 1") {
		t.Errorf("expected 'quarantined: 1' in bar, got: %s", bar)
	}
}

func TestPipelineBarProgress(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "a.md"), "Ready")
	writeIssue(t, filepath.Join(issuesDir, "done", "b.md"), "Done")
	writeIssue(t, filepath.Join(issuesDir, "done", "c.md"), "Done2")

	m := NewModel(config.Config{IssueDir: issuesDir})
	bar := m.pipelineBar()

	if !strings.Contains(bar, "67%") && !strings.Contains(bar, "66%") {
		t.Errorf("expected ~67%% progress for 2/3 done, got: %s", bar)
	}
}

func TestPipelineBarAllDone(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "done", "a.md"), "Done1")
	writeIssue(t, filepath.Join(issuesDir, "done", "b.md"), "Done2")

	m := NewModel(config.Config{IssueDir: issuesDir})
	bar := m.pipelineBar()

	if !strings.Contains(bar, "100%") {
		t.Errorf("expected 100%% progress when all done, got: %s", bar)
	}
}

func TestPipelineBarNoneDone(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "todo.md"), "Todo")
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "a.md"), "Ready")

	m := NewModel(config.Config{IssueDir: issuesDir})
	bar := m.pipelineBar()

	if !strings.Contains(bar, "0%") {
		t.Errorf("expected 0%% progress when nothing done, got: %s", bar)
	}
}

func TestPipelineBarOnlyQuarantined(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, ".quarantine", "a.md"), "Quar1")

	m := NewModel(config.Config{IssueDir: issuesDir})
	bar := m.pipelineBar()

	if strings.Contains(bar, "no issues found") {
		t.Errorf("expected bar to render with quarantined issues, got: %s", bar)
	}
	if !strings.Contains(bar, "quarantined: 1") {
		t.Errorf("expected quarantined count in bar, got: %s", bar)
	}
}

func TestViewShowsUnparseableWarning(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)

	os.WriteFile(filepath.Join(issuesDir, "test-ready", "empty.md"), []byte{}, 0644)
	os.WriteFile(filepath.Join(issuesDir, "test-ready", "good.md"), []byte("# Good\n\nBody"), 0644)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	if len(m.warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(m.warnings))
	}

	view := m.View()
	if !strings.Contains(view, "empty.md (unparseable)") {
		t.Errorf("expected unparseable filename in view, got:\n%s", view)
	}
}

func TestViewNoUnparseableWarning(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)

	os.WriteFile(filepath.Join(issuesDir, "test-ready", "good.md"), []byte("# Good\n\nBody"), 0644)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	if len(m.warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %d", len(m.warnings))
	}

	view := m.View()
	if strings.Contains(view, "(unparseable)") {
		t.Errorf("unexpected unparseable marker in view:\n%s", view)
	}
}

func TestViewShowsTransitions(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	issue.AppendTransitionLog(issuesDir, issue.TransitionEvent{Title: "Test Issue", From: "todo", To: "done"})

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	view := m.View()
	if !strings.Contains(view, "Recent transitions") {
		t.Error("expected 'Recent transitions' in view")
	}
	if !strings.Contains(view, "Test Issue") {
		t.Error("expected 'Test Issue' in view")
	}
	if !strings.Contains(view, "todo → done") {
		t.Error("expected transition direction in view")
	}
}

func TestUpdateNonExistentKeyDoesNotChangeState(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.page = pageTodo

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.quit {
		t.Error("expected quit to be false for unbound key")
	}
	if m2.page != pageTodo {
		t.Errorf("expected page to remain pageTodo, got %d", m2.page)
	}
}

func TestNavigateRightArrow(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m2 := result.(Model)
	if m2.page != pageTodo {
		t.Errorf("right arrow: expected pageTodo, got %d", m2.page)
	}
}

func TestNavigateLeftArrow(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.page = pageDone

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m2 := result.(Model)
	if m2.page != pageTestReady {
		t.Errorf("left arrow: expected pageTestReady, got %d", m2.page)
	}
}

func TestNewModelStoresConfig(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir, Repo: "my-org/my-repo"}
	m := NewModel(cfg)

	if m.cfg.Repo != "my-org/my-repo" {
		t.Errorf("expected repo 'my-org/my-repo', got %q", m.cfg.Repo)
	}
	if m.cfg.IssueDir != issuesDir {
		t.Errorf("expected IssueDir %q, got %q", issuesDir, m.cfg.IssueDir)
	}
}

func TestPipelineBarShowsDashWhenNoActiveIssues(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, ".quarantine", "a.md"), "Only Quar")

	m := NewModel(config.Config{IssueDir: issuesDir})
	bar := m.pipelineBar()

	if strings.Contains(bar, "NaN") {
		t.Error("expected no NaN in pipeline bar")
	}
	if strings.Contains(bar, "0%") {
		t.Error("expected no 0% when total is 0 (only quarantined)")
	}
}

func TestHeaderShowsVersion(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	m := NewModel(config.Config{IssueDir: issuesDir})
	m.Version = "1.2.3"
	header := m.headerView()

	if !strings.Contains(header, "v1.2.3") {
		t.Errorf("expected version v1.2.3 in header, got: %s", header)
	}
}

func TestHeaderDefaultsToDev(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	m := NewModel(config.Config{IssueDir: issuesDir})
	header := m.headerView()

	if !strings.Contains(header, "vdev") {
		t.Errorf("expected vdev in header when version is empty, got: %s", header)
	}
}

func TestHeaderShowsRepo(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	m := NewModel(config.Config{IssueDir: issuesDir, Repo: "my-org/my-repo"})
	header := m.headerView()

	if !strings.Contains(header, "my-org/my-repo") {
		t.Errorf("expected repo in header, got: %s", header)
	}
}

func TestHeaderDefaultsToLocal(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	m := NewModel(config.Config{IssueDir: issuesDir})
	header := m.headerView()

	if !strings.Contains(header, "local") {
		t.Errorf("expected 'local' in header when repo is empty, got: %s", header)
	}
}

func TestHeaderShowsIterationCounter(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "a.md"), "Todo item")
	writeIssue(t, filepath.Join(issuesDir, "test-ready", "b.md"), "Ready item")
	writeIssue(t, filepath.Join(issuesDir, "done", "c.md"), "Done item")

	m := NewModel(config.Config{IssueDir: issuesDir})
	header := m.headerView()

	if !strings.Contains(header, "1/3") {
		t.Errorf("expected iteration '1/3' (1 done / 3 total), got: %s", header)
	}
}

func TestHeaderShowsDashWhenNoIssues(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	m := NewModel(config.Config{IssueDir: issuesDir})
	header := m.headerView()

	if !strings.Contains(header, "·  -") {
		t.Errorf("expected dash when no issues, got: %s", header)
	}
}

func TestScreenshotKeySavesFile(t *testing.T) {
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)
	os.Chdir(dir)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	result, cmd := m.Update(msg)
	m2 := result.(Model)

	if m2.screenshotSaved == "" {
		t.Error("expected screenshotSaved to be set after pressing 's'")
	}
	if !strings.Contains(m2.screenshotSaved, "screenshot saved") && !strings.Contains(m2.screenshotSaved, "screenshot error") {
		t.Errorf("expected screenshot message, got %q", m2.screenshotSaved)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (clear timer)")
	}

	if strings.Contains(m2.screenshotSaved, "screenshot saved") {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "loop-screenshot-") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected a loop-screenshot- file to exist")
		}
	}
}

func TestScreenshotClearedByTimer(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.screenshotSaved = "screenshot saved: loop-screenshot-test.txt"

	msg := screenshotMsg("")
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.screenshotSaved != "" {
		t.Error("expected screenshotSaved to be cleared")
	}
}

func TestScreenshotShownInView(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	m.screenshotSaved = "screenshot saved: test.txt"

	view := m.View()
	if !strings.Contains(view, "screenshot saved: test.txt") {
		t.Errorf("expected screenshot message in view, got:\n%s", view)
	}
}

func TestScreenshotHelpText(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)
	view := m.View()

	if !strings.Contains(view, "s screenshot") {
		t.Errorf("expected 's screenshot' in help text, got:\n%s", view)
	}
}

func TestScreenshotKeyDoesNotQuit(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	result, _ := m.Update(msg)
	m2 := result.(Model)

	if m2.quit {
		t.Error("expected quit to be false after screenshot key")
	}
}

func TestHeaderAllDone(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	setupIssueDirs(t, issuesDir)
	writeIssue(t, filepath.Join(issuesDir, "done", "a.md"), "Done1")
	writeIssue(t, filepath.Join(issuesDir, "done", "b.md"), "Done2")
	writeIssue(t, filepath.Join(issuesDir, "done", "c.md"), "Done3")

	m := NewModel(config.Config{IssueDir: issuesDir})
	header := m.headerView()

	if !strings.Contains(header, "3/3") {
		t.Errorf("expected iteration '3/3' when all done, got: %s", header)
	}
}
