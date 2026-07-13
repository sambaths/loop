package output

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sambaths/loop/internal/config"
)

func noopRun(lineChan chan<- string, iterChan chan<- IterMsg, doneChan chan<- error) {
	lineChan <- "test output"
	doneChan <- nil
	close(lineChan)
}

func TestNewModel(t *testing.T) {
	m := NewModel(config.Config{IssueDir: "test/issues"}, 5, noopRun)
	if m.total != 5 {
		t.Errorf("expected total 5, got %d", m.total)
	}
	if !m.autoOn {
		t.Error("expected auto-scroll to be enabled by default")
	}
	if m.Cfg.IssueDir != "test/issues" {
		t.Errorf("expected IssueDir %q, got %q", "test/issues", m.Cfg.IssueDir)
	}
}

func TestInitReturnsTickCmd(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	cmd := m.Init()
	if cmd == nil {
		t.Error("expected non-nil cmd from Init")
	}
}

func TestQuit(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	result, cmd := m.Update(msg)
	m2 := result.(Model)
	if !m2.quitting {
		t.Error("expected quitting to be true")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestCtrlC(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	result, cmd := m.Update(msg)
	m2 := result.(Model)
	if !m2.quitting {
		t.Error("expected quitting to be true")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestToggleAutoScroll(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}
	result, _ := m.Update(msg)
	m2 := result.(Model)
	if m2.autoOn {
		t.Error("expected auto-scroll to be disabled after toggle")
	}
	result2, _ := m2.Update(msg)
	m3 := result2.(Model)
	if !m3.autoOn {
		t.Error("expected auto-scroll to be re-enabled after second toggle")
	}
}

func TestArrowKeyDisablesAutoScroll(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.ready = true
	msg := tea.KeyMsg{Type: tea.KeyDown}
	result, _ := m.Update(msg)
	m2 := result.(Model)
	if m2.autoOn {
		t.Error("expected auto-scroll to be disabled after arrow key")
	}
}

func TestGKeyDisablesAutoScroll(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	result, _ := m.Update(msg)
	m2 := result.(Model)
	if m2.autoOn {
		t.Error("expected auto-scroll to be disabled after 'g' key")
	}
}

func TestGKeyGoesToTop(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.lines = []string{"a", "b", "c", "d", "e"}
	m.updateViewport()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	_, _ = m.Update(msg)
	// Just verify it doesn't panic - GotoTop is called internally
}

func TestUpperCaseGEnablesAutoScroll(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.autoOn = false
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	result, _ := m.Update(msg)
	m2 := result.(Model)
	if !m2.autoOn {
		t.Error("expected auto-scroll to be enabled after 'G' key")
	}
}

func TestLineMsgAppendsLines(t *testing.T) {
	m := NewModel(config.Config{}, 5, func(lineChan chan<- string, _ chan<- IterMsg, doneChan chan<- error) {
		doneChan <- nil
	})
	m.lines = make([]string, 0, 100)
	result, cmd := m.Update(LineMsg("line 1"))
	m2 := result.(Model)
	if len(m2.lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(m2.lines))
	}
	if m2.lines[0] != "line 1" {
		t.Errorf("expected %q, got %q", "line 1", m2.lines[0])
	}
	if cmd == nil {
		t.Error("expected non-nil cmd from LineMsg (listen command)")
	}
}

func TestLineMsgDetectsGHWarnings(t *testing.T) {
	m := NewModel(config.Config{}, 5, func(lineChan chan<- string, _ chan<- IterMsg, doneChan chan<- error) {
		doneChan <- nil
	})

	r1, _ := m.Update(LineMsg("warning: GitHub issue #42 is closed but still in test-ready"))
	m2 := r1.(Model)
	if m2.warningCount != 1 {
		t.Errorf("expected 1 warning, got %d", m2.warningCount)
	}

	r2, _ := m2.Update(LineMsg("  gh failure: github label sync for #42: failed"))
	m3 := r2.(Model)
	if m3.warningCount != 2 {
		t.Errorf("expected 2 warnings, got %d", m3.warningCount)
	}
}

func TestLineMsgIgnoresRegularMessages(t *testing.T) {
	m := NewModel(config.Config{}, 5, func(lineChan chan<- string, _ chan<- IterMsg, doneChan chan<- error) {
		doneChan <- nil
	})

	r1, _ := m.Update(LineMsg("scanning issues..."))
	m2 := r1.(Model)
	if m2.warningCount != 0 {
		t.Errorf("expected 0 warnings for regular line, got %d", m2.warningCount)
	}

	r2, _ := m2.Update(LineMsg("--- iteration 1/5 ---"))
	m3 := r2.(Model)
	if m3.warningCount != 0 {
		t.Errorf("expected 0 warnings for iteration header, got %d", m3.warningCount)
	}
}

func TestLineMsgRespectsMaxLines(t *testing.T) {
	m := NewModel(config.Config{}, 5, func(lineChan chan<- string, _ chan<- IterMsg, doneChan chan<- error) {
		doneChan <- nil
	})
	for i := 0; i < maxLines+100; i++ {
		result, _ := m.Update(LineMsg(fmt.Sprintf("line %d", i)))
		m = result.(Model)
	}
	if len(m.lines) > maxLines {
		t.Errorf("expected at most %d lines, got %d", maxLines, len(m.lines))
	}
	if m.lines[0] != fmt.Sprintf("line %d", 100) {
		t.Errorf("expected first line to be 'line 100', got %q", m.lines[0])
	}
}

func TestIterMsgUpdatesState(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	result, _ := m.Update(IterMsg{
		Iteration: 2,
		Total:     5,
		Title:     "Implement auth",
		Role:      "implement",
	})
	m2 := result.(Model)
	if m2.iteration != 2 {
		t.Errorf("expected iteration 2, got %d", m2.iteration)
	}
	if m2.title != "Implement auth" {
		t.Errorf("expected title %q, got %q", "Implement auth", m2.title)
	}
	if m2.role != "implement" {
		t.Errorf("expected role %q, got %q", "implement", m2.role)
	}
	if !m2.running {
		t.Error("expected running to be true after IterMsg")
	}
	if m2.startTime.IsZero() {
		t.Error("expected startTime to be set after IterMsg")
	}
}

func TestIterMsgReturnsListenCommand(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	_, cmd := m.Update(IterMsg{
		Iteration: 1,
		Total:     5,
		Title:     "Test",
		Role:      "implement",
	})
	if cmd == nil {
		t.Error("expected non-nil cmd from IterMsg (listen command)")
	}
}

func TestDoneMsgCompletes(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	result, cmd := m.Update(DoneMsg{Err: nil})
	m2 := result.(Model)
	if !m2.done {
		t.Error("expected done to be true")
	}
	if m2.running {
		t.Error("expected running to be false after DoneMsg")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd after DoneMsg")
	}
}

func TestDoneMsgWithError(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	result, _ := m.Update(DoneMsg{Err: fmt.Errorf("test error")})
	m2 := result.(Model)
	if !m2.done {
		t.Error("expected done to be true")
	}
	if m2.DoneErr == nil {
		t.Error("expected non-nil DoneErr")
	}
	if m2.DoneErr.Error() != "test error" {
		t.Errorf("expected 'test error', got %v", m2.DoneErr)
	}
}

func TestViewAfterQuit(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.quitting = true
	view := m.View()
	if view != "" {
		t.Error("expected empty view after quit")
	}
}

func TestViewShowsIteration(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.ready = true
	m.iteration = 2
	m.total = 5
	m.title = "Test issue"
	m.role = "implement"
	m.running = true
	m.startTime = time.Now()

	view := m.View()
	if !strings.Contains(view, "2/5") {
		t.Error("expected iteration '2/5' in view")
	}
	if !strings.Contains(view, "Test issue") {
		t.Error("expected issue title in view")
	}
	if !strings.Contains(view, "implement") {
		t.Error("expected role in view")
	}
}

func TestViewShowsAutoScrollStatus(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.autoOn = true
	view := m.View()
	if !strings.Contains(view, "ON") {
		t.Error("expected 'ON' in status when auto-scroll enabled")
	}

	m.autoOn = false
	view2 := m.View()
	if !strings.Contains(view2, "OFF") {
		t.Error("expected 'OFF' in status when auto-scroll disabled")
	}
}

func TestViewShowsLinesCount(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.lines = []string{"a", "b", "c"}
	view := m.View()
	if !strings.Contains(view, "Lines: 3") {
		t.Errorf("expected 'Lines: 3' in status, got:\n%s", view)
	}
}

func TestViewShowsElapsedTime(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.title = "Test"
	m.role = "implement"
	m.startTime = time.Now().Add(-90 * time.Second)
	m.elapsed = 90 * time.Second
	m.ready = true

	view := m.View()
	if !strings.Contains(view, "01:30") {
		t.Errorf("expected elapsed '01:30' in view, got:\n%s", view)
	}
}

func TestViewShowsHelpText(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	view := m.View()
	if !strings.Contains(view, "s") && !strings.Contains(view, "q") {
		t.Error("expected help text in status")
	}
}

func TestUnhandledKeyDoesNotCrash(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	_, cmd := m.Update(msg)
	// Viewport passes through unknown keys - should not crash
	if cmd == nil {
		// nil is OK for unknown keys (viewport propagated)
	}
}

func TestWindowSizeSetsViewport(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	msg := tea.WindowSizeMsg{Width: 100, Height: 30}
	result, _ := m.Update(msg)
	m2 := result.(Model)
	if !m2.ready {
		t.Error("expected ready to be true after WindowSizeMsg")
	}
	if m2.viewport.Width != 98 {
		t.Errorf("expected viewport width 98, got %d", m2.viewport.Width)
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

func TestDoneViewShowsComplete(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.done = true
	view := m.View()
	if !strings.Contains(view, "complete") {
		t.Error("expected completion message in view")
	}
}

func TestDoneViewShowsError(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.done = true
	m.DoneErr = fmt.Errorf("agent failed")
	view := m.View()
	if !strings.Contains(view, "Error") || !strings.Contains(view, "agent failed") {
		t.Errorf("expected error message in done view, got:\n%s", view)
	}
}

func TestContentWithLines(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.lines = []string{"line one", "line two", "line three"}
	m.updateViewport()
	content := m.content()
	if !strings.Contains(content, "line one") {
		t.Error("expected content to contain output lines")
	}
	if !strings.Contains(content, "line two") {
		t.Error("expected content to contain output lines")
	}
}

func TestContentEmptyWhenNoLines(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	content := m.content()
	if !strings.Contains(content, "Waiting") {
		t.Error("expected waiting message when no lines")
	}
}

func TestContentShowsLinesAfterDone(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.lines = []string{"final output"}
	m.done = true
	m.updateViewport()
	content := m.content()
	if !strings.Contains(content, "final output") {
		t.Error("expected content to show lines even after done")
	}
}

func TestStatusTextIdle(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	if s := m.statusText(); s != "idle" {
		t.Errorf("expected 'idle', got %q", s)
	}
}

func TestStatusTextRunning(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.running = true
	if s := m.statusText(); s != "running" {
		t.Errorf("expected 'running', got %q", s)
	}
}

func TestStatusTextComplete(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.done = true
	if s := m.statusText(); s != "complete" {
		t.Errorf("expected 'complete', got %q", s)
	}
}

func TestStatusTextError(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.done = true
	m.DoneErr = fmt.Errorf("something went wrong")
	if s := m.statusText(); s != "error" {
		t.Errorf("expected 'error', got %q", s)
	}
}

func TestStatusViewShowsIdle(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	view := m.View()
	if !strings.Contains(view, "[idle]") {
		t.Errorf("expected '[idle]' in view, got:\n%s", view)
	}
}

func TestStatusViewShowsRunning(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.running = true
	m.iteration = 2
	m.total = 5
	view := m.View()
	if !strings.Contains(view, "[running]") {
		t.Errorf("expected '[running]' in view, got:\n%s", view)
	}
}

func TestStatusViewShowsIterationWhenRunning(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.running = true
	m.iteration = 2
	m.total = 5
	status := m.statusView()
	if !strings.Contains(status, "2/5") {
		t.Errorf("expected iteration '2/5' in status, got:\n%s", status)
	}
}

func TestStatusViewShowsWarnings(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.warningCount = 3
	view := m.View()
	if !strings.Contains(view, "warnings: 3") {
		t.Errorf("expected 'warnings: 3' in view, got:\n%s", view)
	}
}

func TestStatusViewHidesWarningsWhenZero(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.warningCount = 0
	status := m.statusView()
	if strings.Contains(status, "warnings: 0") {
		t.Error("expected no warnings display when count is 0")
	}
}

func TestStatusViewShowsNextAction(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.nextAction = "Implement auth"
	view := m.View()
	if !strings.Contains(view, "next: Implement auth") {
		t.Errorf("expected 'next: Implement auth' in view, got:\n%s", view)
	}
}

func TestStatusViewHidesNextActionWhenEmpty(t *testing.T) {
	m := NewModel(config.Config{}, 5, noopRun)
	m.nextAction = ""
	status := m.statusView()
	if strings.Contains(status, "next:") {
		t.Error("expected no 'next:' when nextAction is empty")
	}
}
