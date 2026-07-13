package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sambaths/loop/internal/config"
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
	m := NewModel(cfg).(model)

	if m.page != pageOverview {
		t.Errorf("expected pageOverview, got %d", m.page)
	}
	if m.title != "loop status" {
		t.Errorf("expected title 'loop status', got %q", m.title)
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
	m := NewModel(cfg).(model)

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
	m := NewModel(cfg).(model)

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

func TestNewModelWithRepo(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir, Repo: "my-org/my-repo"}
	m := NewModel(cfg).(model)

	if m.cfg.Repo != "my-org/my-repo" {
		t.Errorf("expected repo 'my-org/my-repo', got %q", m.cfg.Repo)
	}
}

func TestInitReturnsNil(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg).(model)
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
	m := NewModel(cfg).(model)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, cmd := m.Update(msg)
	m2 := result.(model)

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
	m := NewModel(cfg).(model)

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, cmd := m.Update(msg)
	m2 := result.(model)

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
	m := NewModel(cfg).(model)

	m.page = pageOverview
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := result.(model)
	if m2.page != pageTodo {
		t.Errorf("tab: expected pageTodo, got %d", m2.page)
	}

	m.page = pageOverview
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m2 = result.(model)
	if m2.page != pageTodo {
		t.Errorf("n: expected pageTodo, got %d", m2.page)
	}

	m.page = pageOverview
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m2 = result.(model)
	if m2.page != pageTodo {
		t.Errorf("l: expected pageTodo, got %d", m2.page)
	}
}

func TestNavigateBack(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg).(model)

	m.page = pageTestReady
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m2 := result.(model)
	if m2.page != pageTodo {
		t.Errorf("shift+tab: expected pageTodo, got %d", m2.page)
	}

	m.page = pageTestReady
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m2 = result.(model)
	if m2.page != pageTodo {
		t.Errorf("p: expected pageTodo, got %d", m2.page)
	}

	m.page = pageTestReady
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m2 = result.(model)
	if m2.page != pageTodo {
		t.Errorf("h: expected pageTodo, got %d", m2.page)
	}
}

func TestNextStaysAtLastPage(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg).(model)
	m.page = pageQuarantine

	msg := tea.KeyMsg{Type: tea.KeyTab}
	result, _ := m.Update(msg)
	m2 := result.(model)

	if m2.page != pageQuarantine {
		t.Errorf("expected to stay on pageQuarantine, got %d", m2.page)
	}
}

func TestPrevStaysAtFirstPage(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg).(model)

	msg := tea.KeyMsg{Type: tea.KeyShiftTab}
	result, _ := m.Update(msg)
	m2 := result.(model)

	if m2.page != pageOverview {
		t.Errorf("expected to stay on pageOverview, got %d", m2.page)
	}
}

func TestViewOverviewEmpty(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg).(model)
	view := m.View()

	if !strings.Contains(view, "no issues found") {
		t.Error("expected overview to say 'no issues found'")
	}
	if !strings.Contains(view, "issues dir") {
		t.Error("expected overview to show issues dir")
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
	m := NewModel(cfg).(model)
	view := m.View()

	if !strings.Contains(view, "Pipeline overview") {
		t.Error("expected overview header")
	}
	if !strings.Contains(view, "todo") || !containsCount(view, len(m.todo)) {
		t.Error("expected todo count in overview")
	}
	if !strings.Contains(view, "test-ready") || !containsCount(view, len(m.testReady)) {
		t.Error("expected test-ready count in overview")
	}
	if !strings.Contains(view, "done") || !containsCount(view, len(m.done)) {
		t.Error("expected done count in overview")
	}
	if !strings.Contains(view, "quarantined") || !containsCount(view, len(m.quarantined)) {
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
	m := NewModel(cfg).(model)
	view := m.View()

	if !strings.Contains(view, "repo") {
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
	m := NewModel(cfg).(model)
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
	m := NewModel(cfg).(model)
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
	m := NewModel(cfg).(model)
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
	m := NewModel(cfg).(model)
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
	m := NewModel(cfg).(model)
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
	m := NewModel(cfg).(model)
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
	m := NewModel(cfg).(model)
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
	m := NewModel(cfg).(model)
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
	m := NewModel(cfg).(model)
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
	m := NewModel(config.Config{IssueDir: issuesDir}).(model)
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
	m := NewModel(cfg).(model)
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

	m := NewModel(config.Config{IssueDir: issuesDir}).(model)
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

	m := NewModel(config.Config{IssueDir: issuesDir}).(model)
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

	m := NewModel(config.Config{IssueDir: issuesDir}).(model)
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

	m := NewModel(config.Config{IssueDir: issuesDir}).(model)
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
	m := NewModel(cfg).(model)

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
	m := NewModel(cfg).(model)

	if len(m.warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %d", len(m.warnings))
	}

	view := m.View()
	if strings.Contains(view, "(unparseable)") {
		t.Errorf("unexpected unparseable marker in view:\n%s", view)
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
	m := NewModel(cfg).(model)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	result, cmd := m.Update(msg)
	m2 := result.(model)

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

func TestStatusScreenshotClearedByTimer(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg).(model)
	m.screenshotSaved = "screenshot saved: test.txt"

	msg := screenshotMsg("")
	result, _ := m.Update(msg)
	m2 := result.(model)

	if m2.screenshotSaved != "" {
		t.Error("expected screenshotSaved to be cleared")
	}
}

func TestStatusScreenshotShownInView(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg).(model)
	m.screenshotSaved = "screenshot saved: test.txt"

	view := m.View()
	if !strings.Contains(view, "screenshot saved: test.txt") {
		t.Errorf("expected screenshot message in view, got:\n%s", view)
	}
}

func TestStatusScreenshotHelpText(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "issues")
	os.MkdirAll(issuesDir, 0755)

	cfg := config.Config{IssueDir: issuesDir}
	m := NewModel(cfg).(model)
	view := m.View()

	if !strings.Contains(view, "s screenshot") {
		t.Errorf("expected 's screenshot' in help text, got:\n%s", view)
	}
}

func containsCount(s string, count int) bool {
	return strings.Contains(s, fmt.Sprintf("%d", count))
}
