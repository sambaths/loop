package run

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sambaths/loop/internal/config"
)

const maxLogLines = 20

// ProgressMsg reports an iteration progress update.
type ProgressMsg struct {
	Iteration  int
	Total      int
	IssueTitle string
	IssueRole  string
	Phase      string
	Detail     string
}

// LogMsg appends a log line to the model.
type LogMsg struct {
	Text string
}

// CompletionMsg signals that the run loop has finished.
type CompletionMsg struct {
	Err error
}

// tickMsg is sent periodically to update the elapsed timer.
type tickMsg time.Time

// doneWaitingMsg is used internally when a channel closes before DoneMsg.
type doneWaitingMsg struct{}

type Model struct {
	cfg     config.Config
	maxIter int

	iteration int
	total     int
	title     string
	role      string
	phase     string
	detail    string
	logs      []string
	warnings  []string
	Err       error
	Finished  bool
	quit      bool

	startTime time.Time
	elapsed   time.Duration

	// streaming channels
	logChan    chan string
	iterChan   chan ProgressMsg
	doneChan   chan error
	startRunFn func()
	cancel     context.CancelFunc
}

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	phaseStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFF00"))
	detailStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	logStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	successStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC66"))
	helpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	warnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8800"))
	countStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFF00"))
	panelStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	labelStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00AAFF"))
	roleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF8800"))
	timerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFAA"))
	issueNameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
)

func NewModel(cfg config.Config, maxIter int) tea.Model {
	return &Model{
		cfg:     cfg,
		maxIter: maxIter,
		total:   maxIter,
	}
}

// NewStreamingModel creates a model that runs the pipeline in a goroutine and
// streams progress updates and log lines to the Bubbletea event loop.
// The cancel function is called when the user quits (q/ctrl+c) to stop the
// agent subprocess. May be nil.
func NewStreamingModel(cfg config.Config, maxIter int, cancel context.CancelFunc, startRun func(logChan chan<- string, iterChan chan<- ProgressMsg, doneChan chan<- error)) tea.Model {
	logChan := make(chan string, 200)
	iterChan := make(chan ProgressMsg, 10)
	doneChan := make(chan error, 1)

	return &Model{
		cfg:        cfg,
		maxIter:    maxIter,
		total:      maxIter,
		logChan:    logChan,
		iterChan:   iterChan,
		doneChan:   doneChan,
		startRunFn: func() { startRun(logChan, iterChan, doneChan) },
		cancel:     cancel,
	}
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.tick()}
	if m.startRunFn != nil {
		go m.startRunFn()
		cmds = append(cmds, m.listen())
	}
	return tea.Batch(cmds...)
}

func (m *Model) listen() tea.Cmd {
	return func() tea.Msg {
		select {
		case line, ok := <-m.logChan:
			if !ok {
				return doneWaitingMsg{}
			}
			return LogMsg{Text: line}
		case iter, ok := <-m.iterChan:
			if !ok {
				return doneWaitingMsg{}
			}
			return iter
		case err := <-m.doneChan:
			return CompletionMsg{Err: err}
		}
	}
}

func (m *Model) waitDone() tea.Cmd {
	return func() tea.Msg {
		err := <-m.doneChan
		return CompletionMsg{Err: err}
	}
}

func (m *Model) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			m.quit = true
			return m, tea.Quit
		}
	case tickMsg:
		if !m.Finished && !m.quit && !m.startTime.IsZero() {
			m.elapsed = time.Since(m.startTime)
		}
		return m, m.tick()
	case ProgressMsg:
		m.iteration = msg.Iteration
		m.total = msg.Total
		m.title = msg.IssueTitle
		m.role = msg.IssueRole
		m.phase = msg.Phase
		m.detail = msg.Detail
		if m.startTime.IsZero() {
			m.startTime = time.Now()
		}
		return m, m.listen()
	case LogMsg:
		m.logs = append(m.logs, msg.Text)
		if len(m.logs) > maxLogLines {
			m.logs = m.logs[len(m.logs)-maxLogLines:]
		}
		if isGHWarning(msg.Text) {
			m.warnings = append(m.warnings, msg.Text)
		}
		return m, m.listen()
	case CompletionMsg:
		m.Finished = true
		m.Err = msg.Err
		return m, tea.Quit
	case doneWaitingMsg:
		return m, m.waitDone()
	}
	return m, nil
}

func (m *Model) View() string {
	if m.quit {
		return ""
	}
	if m.Finished {
		return m.completionView()
	}
	return m.progressView()
}

// isGHWarning detects GitHub-related warnings and failures in log messages.
func isGHWarning(text string) bool {
	return strings.HasPrefix(text, "warning:") ||
		strings.Contains(text, "gh failure:") ||
		strings.Contains(text, "github failure")
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func (m *Model) Iteration() int { return m.iteration }
func (m *Model) Total() int     { return m.total }

func (m *Model) progressView() string {
	s := titleStyle.Render("loop run") + "\n\n"

	iterStr := fmt.Sprintf("%d", m.iteration)
	if m.total > 0 {
		iterStr = fmt.Sprintf("%d/%d", m.iteration, m.total)
	}
	s += "Iteration " + countStyle.Render(iterStr) + "\n\n"

	// Active iteration panel.
	var panel strings.Builder
	panel.WriteString(labelStyle.Render("Issue") + "\n")
	panel.WriteString("  " + issueNameStyle.Render(m.title) + "\n\n")
	panel.WriteString(labelStyle.Render("Role") + "\n")
	panel.WriteString("  " + roleStyle.Render(m.role) + "\n\n")
	panel.WriteString(labelStyle.Render("Elapsed") + "\n")
	panel.WriteString("  " + timerStyle.Render(formatDuration(m.elapsed)) + "\n")
	s += panelStyle.Render(panel.String()) + "\n\n"

	if m.phase != "" {
		s += phaseStyle.Render(m.phase)
		if m.detail != "" {
			s += "  " + detailStyle.Render(m.detail)
		}
		s += "\n\n"
	}

	if len(m.warnings) > 0 {
		s += warnStyle.Render(fmt.Sprintf("gh warnings: %d", len(m.warnings))) + "\n\n"
	}

	for _, l := range m.logs {
		s += logStyle.Render(l) + "\n"
	}

	s += "\n" + helpStyle.Render("q to quit")
	return s
}

func (m *Model) completionView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("loop run"))
	b.WriteString("\n\n")
	if m.Err != nil {
		b.WriteString(errorStyle.Render("Error: " + m.Err.Error()))
		b.WriteString("\n")
	} else {
		b.WriteString(successStyle.Render("Run complete"))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	iterStr := fmt.Sprintf("Iterations completed: %d", m.iteration)
	if m.total > 0 {
		iterStr = fmt.Sprintf("Iterations completed: %d/%d", m.iteration, m.total)
	}
	b.WriteString(iterStr)
	b.WriteString("\n")
	return b.String()
}
