package run

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sambaths/loop/internal/config"
)

func TestNewModel(t *testing.T) {
	cfg := config.Config{IssueDir: "test/issues"}
	m := NewModel(cfg, 5).(*Model)

	if m.maxIter != 5 {
		t.Errorf("expected maxIter 5, got %d", m.maxIter)
	}
	if m.total != 5 {
		t.Errorf("expected total 5, got %d", m.total)
	}
	if m.cfg.IssueDir != "test/issues" {
		t.Errorf("expected IssueDir %q, got %q", "test/issues", m.cfg.IssueDir)
	}
}

func TestInitReturnsTickCmd(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	cmd := m.Init()
	if cmd == nil {
		t.Error("expected non-nil tick cmd from Init")
	}
}

func TestQuit(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, cmd := m.Update(msg)
	m2 := result.(*Model)

	if !m2.quit {
		t.Error("expected quit to be true")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestCtrlC(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, cmd := m.Update(msg)
	m2 := result.(*Model)

	if !m2.quit {
		t.Error("expected quit to be true")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestProgressUpdate(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	msg := ProgressMsg{
		Iteration:  1,
		Total:      5,
		IssueTitle: "Implement auth",
		IssueRole:  "implement",
		Phase:      "running",
		Detail:     "Running opencode agent...",
	}
	result, _ := m.Update(msg)
	m2 := result.(*Model)

	if m2.iteration != 1 {
		t.Errorf("expected iteration 1, got %d", m2.iteration)
	}
	if m2.title != "Implement auth" {
		t.Errorf("expected title %q, got %q", "Implement auth", m2.title)
	}
	if m2.role != "implement" {
		t.Errorf("expected role %q, got %q", "implement", m2.role)
	}
	if m2.phase != "running" {
		t.Errorf("expected phase %q, got %q", "running", m2.phase)
	}
	if m2.detail != "Running opencode agent..." {
		t.Errorf("expected detail %q, got %q", "Running opencode agent...", m2.detail)
	}
}

func TestProgressOverwritesPrevious(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)

	result1, _ := m.Update(ProgressMsg{
		Iteration:  1,
		Total:      5,
		IssueTitle: "First issue",
		Phase:      "scanning",
	})
	m2 := result1.(*Model)

	result2, _ := m2.Update(ProgressMsg{
		Iteration:  2,
		Total:      5,
		IssueTitle: "Second issue",
		Phase:      "running",
	})
	m3 := result2.(*Model)

	if m3.title != "Second issue" {
		t.Errorf("expected title %q, got %q", "Second issue", m3.title)
	}
	if m3.iteration != 2 {
		t.Errorf("expected iteration 2, got %d", m3.iteration)
	}
	if m3.phase != "running" {
		t.Errorf("expected phase %q, got %q", "running", m3.phase)
	}
}

func TestLogAppend(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	r1, _ := m.Update(LogMsg{Text: "scanning issues..."})
	r2, _ := r1.(*Model).Update(LogMsg{Text: "selected issue: Test"})
	r3, _ := r2.(*Model).Update(LogMsg{Text: "running agent..."})
	m4 := r3.(*Model)

	if len(m4.logs) != 3 {
		t.Errorf("expected 3 log entries, got %d", len(m4.logs))
	}
	if m4.logs[0] != "scanning issues..." {
		t.Errorf("expected %q, got %q", "scanning issues...", m4.logs[0])
	}
}

func TestLogPreservesAllEntries(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	for i := 0; i < 25; i++ {
		r, _ := m.Update(LogMsg{Text: fmt.Sprintf("log %d", i)})
		m = r.(*Model)
	}

	if len(m.logs) != 25 {
		t.Errorf("expected 25 log entries, got %d", len(m.logs))
	}
	if m.logs[len(m.logs)-1] != "log 24" {
		t.Errorf("expected last log %q, got %q", "log 24", m.logs[len(m.logs)-1])
	}
	if m.logs[0] != "log 0" {
		t.Errorf("expected first log %q, got %q", "log 0", m.logs[0])
	}
}

func TestCompletionSuccess(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	result, cmd := m.Update(CompletionMsg{Err: nil})
	m2 := result.(*Model)

	if !m2.Finished {
		t.Error("expected finished to be true")
	}
	if m2.Err != nil {
		t.Errorf("expected nil error, got %v", m2.Err)
	}
	if cmd != nil {
		t.Error("expected nil cmd (no tea.Quit) on CompletionMsg")
	}

	view := m2.View()
	if !strings.Contains(view, "Run complete") {
		t.Error("expected 'Run complete' in completion view")
	}
}

func TestCompletionError(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 2

	result, cmd := m.Update(CompletionMsg{Err: fmt.Errorf("something went wrong")})
	m2 := result.(*Model)

	if !m2.Finished {
		t.Error("expected finished to be true")
	}
	if m2.Err == nil {
		t.Error("expected non-nil error")
	}
	if cmd != nil {
		t.Error("expected nil cmd (no tea.Quit) on CompletionMsg")
	}

	view := m2.View()
	if !strings.Contains(view, "Error:") {
		t.Error("expected 'Error:' in completion view")
	}
	if !strings.Contains(view, "something went wrong") {
		t.Error("expected error message in completion view")
	}
}

func TestViewAfterQuit(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.quit = true
	view := m.View()
	if view != "" {
		t.Error("expected empty view after quit")
	}
}

func TestViewShowsIterationProgress(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 1
	m.total = 5

	view := m.View()
	if !strings.Contains(view, "Iteration") {
		t.Error("expected 'Iteration' in view")
	}
	if !strings.Contains(view, "1/5") {
		t.Error("expected '1/5' iteration display in view")
	}
}

func TestViewShowsIssueInfo(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 2
	m.total = 3
	m.title = "Add database"
	m.role = "implement"
	m.phase = "running"

	view := m.View()
	if !strings.Contains(view, "Add database") {
		t.Error("expected issue title in view")
	}
	if !strings.Contains(view, "implement") {
		t.Error("expected role in view")
	}
}

func TestViewShowsPhase(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 1
	m.total = 1
	m.phase = "scanning"
	m.detail = "looking for issues..."

	view := m.View()
	if !strings.Contains(view, "scanning") {
		t.Error("expected phase in view")
	}
	if !strings.Contains(view, "looking for issues...") {
		t.Error("expected detail in view")
	}
}

func TestViewShowsLogs(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 1
	m.total = 3
	m.logs = []string{"scanning issues...", "selected: Implement auth", "running agent..."}
	m.updateLogViewport()

	view := m.View()
	if !strings.Contains(view, "scanning issues...") {
		t.Error("expected log entry 'scanning issues...' in view")
	}
	if !strings.Contains(view, "selected: Implement auth") {
		t.Error("expected log entry 'selected: Implement auth' in view")
	}
}

func TestViewEmptyInitialState(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view for initial state")
	}
}

func TestViewHelpText(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 1
	m.total = 3
	view := m.View()

	if !strings.Contains(view, "q to quit") {
		t.Error("expected help text 'q to quit' in view")
	}
	if !strings.Contains(view, "Auto-scroll:") {
		t.Error("expected 'Auto-scroll:' in view")
	}
}

func TestViewIterationWithoutTotal(t *testing.T) {
	m := NewModel(config.Config{}, 0).(*Model)
	m.iteration = 3
	view := m.View()

	if strings.Contains(view, "3/0") {
		t.Error("expected iteration not to show /0 format when total is 0")
	}
	if !strings.Contains(view, "3") {
		t.Error("expected to show iteration number")
	}
}

func TestCompletionShowsCount(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	r, _ := m.Update(CompletionMsg{})
	m2 := r.(*Model)

	view := m2.View()
	if !strings.Contains(view, "3/5") {
		t.Errorf("expected completion view to show '3/5', got:\n%s", view)
	}
	if !strings.Contains(view, "Iterations:") {
		t.Error("expected 'Iterations:' label in completion view")
	}
}

func TestCompletionWithoutTotal(t *testing.T) {
	m := NewModel(config.Config{}, 0).(*Model)
	m.iteration = 3

	r, _ := m.Update(CompletionMsg{})
	m2 := r.(*Model)

	view := m2.View()
	if strings.Contains(view, "3/0") {
		t.Errorf("expected completion view to not show /0 format, got:\n%s", view)
	}
	if !strings.Contains(view, "Iterations:") {
		t.Error("expected 'Iterations:' label in completion view")
	}
}

func TestCompletionKeyDismisses(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	result, _ := m.Update(CompletionMsg{Err: nil})
	m2 := result.(*Model)

	if !m2.Finished {
		t.Fatal("expected finished to be true")
	}

	// Any key (q) should dismiss the completion screen.
	result2, cmd := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m3 := result2.(*Model)

	if !m3.quit {
		t.Error("expected quit to be true after key press in completed state")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestCompletionCtrlCDismisses(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	result, _ := m.Update(CompletionMsg{Err: nil})
	m2 := result.(*Model)

	if !m2.Finished {
		t.Fatal("expected finished to be true")
	}

	result2, cmd := m2.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m3 := result2.(*Model)

	if !m3.quit {
		t.Error("expected quit to be true after ctrl+c in completed state")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestCompletionEnterDismisses(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	result, _ := m.Update(CompletionMsg{Err: nil})
	m2 := result.(*Model)

	if !m2.Finished {
		t.Fatal("expected finished to be true")
	}

	result2, cmd := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := result2.(*Model)

	if !m3.quit {
		t.Error("expected quit to be true after Enter in completed state")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestCompletionScrollKeysDoNotDismiss(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	result, _ := m.Update(CompletionMsg{Err: nil})
	m2 := result.(*Model)

	if !m2.Finished {
		t.Fatal("expected finished to be true")
	}

	// Scroll keys should NOT dismiss.
	scrollKeys := []tea.KeyType{tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown}
	for _, kt := range scrollKeys {
		r, _ := m2.Update(tea.KeyMsg{Type: kt})
		m3 := r.(*Model)
		if m3.quit {
			t.Errorf("expected quit to be false after scroll key %v in completed state", kt)
		}
	}
}

func TestCompletionGKeyDoesNotDismiss(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	result, _ := m.Update(CompletionMsg{Err: nil})
	m2 := result.(*Model)

	if !m2.Finished {
		t.Fatal("expected finished to be true")
	}

	r, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m3 := r.(*Model)
	if m3.quit {
		t.Error("expected quit to be false after g in completed state")
	}
}

func TestCompletionHomeKeyDoesNotDismiss(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	result, _ := m.Update(CompletionMsg{Err: nil})
	m2 := result.(*Model)

	if !m2.Finished {
		t.Fatal("expected finished to be true")
	}

	r, _ := m2.Update(tea.KeyMsg{Type: tea.KeyHome})
	m3 := r.(*Model)
	if m3.quit {
		t.Error("expected quit to be false after Home in completed state")
	}
}

func TestUpdateNonExistentKeyDoesNotChangeState(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 1

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	result, _ := m.Update(msg)
	m2 := result.(*Model)

	if m2.quit {
		t.Error("expected quit to be false for unbound key")
	}
	if m2.iteration != 1 {
		t.Errorf("expected iteration to remain 1, got %d", m2.iteration)
	}
}

func TestLogDetectsGHWarnings(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)

	r1, _ := m.Update(LogMsg{Text: "warning: GitHub issue #42 is closed but still in test-ready"})
	m2 := r1.(*Model)
	if len(m2.warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(m2.warnings))
	}

	r2, _ := m2.Update(LogMsg{Text: "  gh failure: github label sync for #42: something failed"})
	m3 := r2.(*Model)
	if len(m3.warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(m3.warnings))
	}
}

func TestLogIgnoresRegularMessages(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)

	r1, _ := m.Update(LogMsg{Text: "scanning issues..."})
	m2 := r1.(*Model)
	if len(m2.warnings) != 0 {
		t.Errorf("expected 0 warnings for regular log, got %d", len(m2.warnings))
	}

	r2, _ := m2.Update(LogMsg{Text: "--- iteration 1/5 ---"})
	m3 := r2.(*Model)
	if len(m3.warnings) != 0 {
		t.Errorf("expected 0 warnings for iteration header, got %d", len(m3.warnings))
	}
}

func TestViewShowsGHWarningCount(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 1
	m.total = 3
	m.warnings = []string{"warning: test"}

	view := m.View()
	if !strings.Contains(view, "gh warnings: 1") {
		t.Errorf("expected 'gh warnings: 1' in view, got:\n%s", view)
	}
}

func TestViewHidesWarningCountWhenZero(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 1
	m.total = 3

	view := m.View()
	if strings.Contains(view, "gh warnings:") {
		t.Error("expected no warnings display when warning list is empty")
	}
}

func TestProgressPreservesLogs(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	r1, _ := m.Update(LogMsg{Text: "log entry"})
	r2, _ := r1.(*Model).Update(ProgressMsg{Iteration: 1, Total: 5, Phase: "scanning"})
	m3 := r2.(*Model)

	if len(m3.logs) != 1 {
		t.Errorf("expected 1 log entry preserved, got %d", len(m3.logs))
	}
	if m3.logs[0] != "log entry" {
		t.Errorf("expected log entry %q, got %q", "log entry", m3.logs[0])
	}
}

func TestCompletionDoesNotShowProgressBar(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3
	m.title = "Some issue"
	m.phase = "scanning"

	r, _ := m.Update(CompletionMsg{})
	m2 := r.(*Model)

	view := m2.View()
	if !strings.Contains(view, "Run complete") {
		t.Error("expected completion view to show 'Run complete'")
	}
	if strings.Contains(view, "scanning") {
		t.Error("expected completion view to not show phase from before completion")
	}
}

func TestFinishedWithIterationZero(t *testing.T) {
	m := NewModel(config.Config{}, 3).(*Model)
	m.iteration = 0

	r, _ := m.Update(CompletionMsg{})
	m2 := r.(*Model)

	view := m2.View()
	if !strings.Contains(view, "0/3") {
		t.Errorf("expected completion view to show '0/3', got:\n%s", view)
	}
	if !strings.Contains(view, "Iterations:") {
		t.Error("expected 'Iterations:' label in completion view")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d      time.Duration
		expect string
	}{
		{0, "00:00"},
		{time.Second, "00:01"},
		{time.Minute, "01:00"},
		{time.Minute + 30*time.Second, "01:30"},
		{time.Hour, "01:00:00"},
		{2*time.Hour + 15*time.Minute + 5*time.Second, "02:15:05"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.expect {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.expect)
		}
	}
}

func TestTimerStartsOnFirstProgressMsg(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	if !m.startTime.IsZero() {
		t.Error("expected startTime to be zero initially")
	}

	before := time.Now()
	r, _ := m.Update(ProgressMsg{
		Iteration:  1,
		Total:      5,
		IssueTitle: "Test",
		IssueRole:  "implement",
	})
	m2 := r.(*Model)

	if m2.startTime.IsZero() {
		t.Error("expected startTime to be set after ProgressMsg")
	}
	if m2.startTime.Before(before) {
		t.Error("expected startTime to be after or equal to time before update")
	}
}

func TestTimerNotOverwrittenOnSubsequentProgressMsg(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	r1, _ := m.Update(ProgressMsg{
		Iteration:  1,
		Total:      5,
		IssueTitle: "First",
		IssueRole:  "implement",
	})
	m2 := r1.(*Model)
	originalStart := m2.startTime

	r2, _ := m2.Update(ProgressMsg{
		Iteration:  2,
		Total:      5,
		IssueTitle: "Second",
		IssueRole:  "test",
	})
	m3 := r2.(*Model)

	if !m3.startTime.Equal(originalStart) {
		t.Error("expected startTime to not be overwritten by second ProgressMsg")
	}
}

func TestTickUpdatesElapsed(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.startTime = time.Now().Add(-5 * time.Second)
	m.elapsed = 0

	r, _ := m.Update(tickMsg(time.Now()))
	m2 := r.(*Model)

	if m2.elapsed == 0 {
		t.Error("expected elapsed to be non-zero after tick with startTime set")
	}
	if m2.elapsed < 5*time.Second {
		t.Errorf("expected elapsed >= 5s, got %v", m2.elapsed)
	}
}

func TestTickDoesNotUpdateWhenStartTimeIsZero(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.startTime = time.Time{}
	m.elapsed = 0

	r, _ := m.Update(tickMsg(time.Now()))
	m2 := r.(*Model)

	if m2.elapsed != 0 {
		t.Errorf("expected elapsed to remain 0 when startTime is zero, got %v", m2.elapsed)
	}
}

func TestTickDoesNotUpdateWhenFinished(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.startTime = time.Now().Add(-10 * time.Second)
	m.Finished = true

	r, _ := m.Update(tickMsg(time.Now()))
	m2 := r.(*Model)

	if m2.elapsed != 0 {
		t.Errorf("expected elapsed to remain 0 when finished, got %v", m2.elapsed)
	}
}

func TestActiveIterationPanelShowsIssueRoleElapsed(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 2
	m.total = 5
	m.title = "Add login feature"
	m.role = "implement"
	m.startTime = time.Now().Add(-90 * time.Second)
	m.elapsed = 90 * time.Second

	view := m.View()
	if !strings.Contains(view, "Add login feature") {
		t.Error("expected issue name in view")
	}
	if !strings.Contains(view, "implement") {
		t.Error("expected role in view")
	}
	if !strings.Contains(view, "01:30") {
		t.Error("expected elapsed time '01:30' in view")
	}
}

func TestActiveIterationPanelDoesNotShowZeroIssue(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	view := m.View()
	if strings.Contains(view, "Issue\n\n  ") {
		t.Error("expected panel to not show empty issue placeholder")
	}
}

func TestNewStreamingModelStoresCancel(t *testing.T) {
	cancel := func() {}

	m := NewStreamingModel(config.Config{}, 5, cancel, func(logChan chan<- string, iterChan chan<- ProgressMsg, doneChan chan<- error) {
		doneChan <- nil
		close(logChan)
	}).(*Model)

	if m.cancel == nil {
		t.Error("expected cancel to be stored")
	}
}

func TestCancelCalledOnQuit(t *testing.T) {
	var called atomic.Bool
	cancel := func() { called.Store(true) }

	m := NewStreamingModel(config.Config{}, 5, cancel, func(logChan chan<- string, iterChan chan<- ProgressMsg, doneChan chan<- error) {
		// No-op, won't be invoked.
	}).(*Model)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	m.Update(msg)

	if !called.Load() {
		t.Error("expected cancel to be called on q")
	}
}

func TestCancelCalledOnCtrlC(t *testing.T) {
	var calls atomic.Int32
	cancel := func() { calls.Add(1) }

	m := NewStreamingModel(config.Config{}, 5, cancel, func(logChan chan<- string, iterChan chan<- ProgressMsg, doneChan chan<- error) {
		// No-op, won't be invoked.
	}).(*Model)

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	m.Update(msg)

	if calls.Load() != 1 {
		t.Errorf("expected cancel to be called once on ctrl+c, got %d", calls.Load())
	}
}

func TestCancelNotCalledOnOtherKeys(t *testing.T) {
	var called atomic.Bool
	cancel := func() { called.Store(true) }

	m := NewStreamingModel(config.Config{}, 5, cancel, func(logChan chan<- string, iterChan chan<- ProgressMsg, doneChan chan<- error) {
		// No-op.
	}).(*Model)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	m.Update(msg)

	if called.Load() {
		t.Error("expected cancel NOT to be called on other keys")
	}
}

func TestIterationGetter(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3
	if m.Iteration() != 3 {
		t.Errorf("expected Iteration()=3, got %d", m.Iteration())
	}
}

func TestTotalGetter(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	if m.Total() != 5 {
		t.Errorf("expected Total()=5, got %d", m.Total())
	}
}

func TestCancelIsNilSafe(t *testing.T) {
	m := NewStreamingModel(config.Config{}, 5, nil, func(logChan chan<- string, iterChan chan<- ProgressMsg, doneChan chan<- error) {
		doneChan <- nil
		close(logChan)
	}).(*Model)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd := m.Update(msg)

	if !m.quit {
		t.Error("expected quit to be true even with nil cancel")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestAutoScrollEnabledByDefault(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	if !m.autoOn {
		t.Error("expected auto-scroll to be enabled by default")
	}
}

func TestAutoScrollToggle(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)

	r1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m2 := r1.(*Model)
	if m2.autoOn {
		t.Error("expected auto-scroll to be disabled after s")
	}

	r2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m3 := r2.(*Model)
	if !m3.autoOn {
		t.Error("expected auto-scroll to be re-enabled after second s")
	}
}

func TestManualScrollDisablesAutoScroll(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)

	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := r.(*Model)
	if m2.autoOn {
		t.Error("expected auto-scroll to be disabled after manual scroll")
	}
}

func TestGEndReenablesAutoScroll(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.autoOn = false

	r1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m2 := r1.(*Model)
	if !m2.autoOn {
		t.Error("expected auto-scroll to be enabled after G")
	}
}

func TestEndReenablesAutoScroll(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.autoOn = false

	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m2 := r.(*Model)
	if !m2.autoOn {
		t.Error("expected auto-scroll to be enabled after End")
	}
}

func TestHomeDisablesAutoScroll(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)

	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m2 := r.(*Model)
	if m2.autoOn {
		t.Error("expected auto-scroll to be disabled after Home")
	}
}

func TestGDisablesAutoScroll(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)

	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m2 := r.(*Model)
	if m2.autoOn {
		t.Error("expected auto-scroll to be disabled after g")
	}
}

func TestViewportScrollDown(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	for i := 0; i < 50; i++ {
		r, _ := m.Update(LogMsg{Text: fmt.Sprintf("log line %d", i)})
		m = r.(*Model)
	}
	// Scroll to top first (disables auto-scroll)
	m.viewport.GotoTop()
	m.autoOn = false
	initialOffset := m.viewport.YOffset

	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := r.(*Model)

	if m2.viewport.YOffset <= initialOffset {
		t.Error("expected viewport YOffset to increase after scroll down")
	}
}

func TestViewportScrollUp(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	for i := 0; i < 50; i++ {
		r, _ := m.Update(LogMsg{Text: fmt.Sprintf("log line %d", i)})
		m = r.(*Model)
	}
	m.viewport.GotoBottom()
	initialOffset := m.viewport.YOffset

	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m2 := r.(*Model)

	if m2.viewport.YOffset >= initialOffset {
		t.Error("expected viewport YOffset to decrease after scroll up")
	}
}

func TestViewportGotoTop(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	for i := 0; i < 50; i++ {
		r, _ := m.Update(LogMsg{Text: fmt.Sprintf("log line %d", i)})
		m = r.(*Model)
	}
	m.viewport.GotoBottom()

	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m2 := r.(*Model)

	if m2.viewport.YOffset != 0 {
		t.Errorf("expected YOffset 0 after g, got %d", m2.viewport.YOffset)
	}
}

func TestViewportGotoBottom(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	for i := 0; i < 50; i++ {
		r, _ := m.Update(LogMsg{Text: fmt.Sprintf("log line %d", i)})
		m = r.(*Model)
	}
	m.viewport.GotoTop()

	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m2 := r.(*Model)

	if m2.viewport.YOffset != m2.viewport.TotalLineCount()-m2.viewport.Height {
		t.Errorf("expected YOffset at bottom, got %d (total lines: %d, height: %d)",
			m2.viewport.YOffset, m2.viewport.TotalLineCount(), m2.viewport.Height)
	}
}

func TestAutoScrollScrollsToBottomOnNewLog(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	for i := 0; i < 50; i++ {
		r, _ := m.Update(LogMsg{Text: fmt.Sprintf("log line %d", i)})
		m = r.(*Model)
	}
	m.viewport.GotoTop()
	m.autoOn = true

	r, _ := m.Update(LogMsg{Text: "new log line"})
	m2 := r.(*Model)

	expectedBottom := m2.viewport.TotalLineCount() - m2.viewport.Height
	if m2.viewport.YOffset != expectedBottom {
		t.Errorf("expected viewport YOffset %d at bottom, got %d", expectedBottom, m2.viewport.YOffset)
	}
}

func TestAutoScrollViewDisplaysLogLines(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	for i := 0; i < 10; i++ {
		r, _ := m.Update(LogMsg{Text: fmt.Sprintf("visible line %d", i)})
		m = r.(*Model)
	}

	view := m.View()
	for i := 0; i < 10; i++ {
		if !strings.Contains(view, fmt.Sprintf("visible line %d", i)) {
			t.Errorf("expected view to contain 'visible line %d'", i)
		}
	}
}

func TestCompletionEnterDismisses(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	r, _ := m.Update(CompletionMsg{})
	m2 := r.(*Model)

	r2, cmd := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := r2.(*Model)

	if !m3.quit {
		t.Error("expected quit to be true after Enter in completed state")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd after Enter in completed state")
	}
}

func TestCompletionScrollKeysDoNotDismiss(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	r, _ := m.Update(CompletionMsg{})
	m2 := r.(*Model)

	// Down key should NOT dismiss
	r2, cmd := m2.Update(tea.KeyMsg{Type: tea.KeyDown})
	m3 := r2.(*Model)

	if m3.quit {
		t.Error("expected quit to remain false after down key in completed state")
	}
	// Viewport-based keys return a cmd from the viewport update
	if m3.Finished != true {
		t.Error("expected Finished to remain true")
	}
	_ = cmd // viewport cmd may be non-nil, that's fine
}

func TestCompletionViewShowsPipelineCounts(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	r, _ := m.Update(CompletionMsg{})
	m2 := r.(*Model)

	view := m2.View()
	if !strings.Contains(view, "Pipeline counts") {
		t.Errorf("expected completion view to show 'Pipeline counts', got:\n%s", view)
	}
}

func TestCompletionViewShowsPressAnyKey(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3

	r, _ := m.Update(CompletionMsg{})
	m2 := r.(*Model)

	view := m2.View()
	if !strings.Contains(view, "Press any key to exit") {
		t.Errorf("expected completion view to show 'Press any key to exit', got:\n%s", view)
	}
}

func TestCompletionViewShowsElapsed(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3
	m.startTime = time.Now().Add(-2 * time.Minute)
	m.elapsed = 2 * time.Minute

	r, _ := m.Update(CompletionMsg{})
	m2 := r.(*Model)

	view := m2.View()
	if !strings.Contains(view, "Elapsed:") {
		t.Errorf("expected completion view to show 'Elapsed:', got:\n%s", view)
	}
	if !strings.Contains(view, "02:00") {
		t.Errorf("expected completion view to show elapsed time '02:00', got:\n%s", view)
	}
}

func TestCompletionLogViewportStillShown(t *testing.T) {
	m := NewModel(config.Config{}, 5).(*Model)
	m.iteration = 3
	m.logs = []string{"log line 1", "log line 2"}
	m.updateLogViewport()

	r, _ := m.Update(CompletionMsg{})
	m2 := r.(*Model)

	view := m2.View()
	if !strings.Contains(view, "log line 1") {
		t.Errorf("expected completion view to show log lines, got:\n%s", view)
	}
}
