package output

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sambaths/loop/internal/config"
)

const maxLines = 5000

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00AAFF"))
	valueStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	roleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF8800"))
	timerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFAA"))
	scrollStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))
	countStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC66"))
	headerStyle  = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderBottom(true)
	statusStyle  = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderTop(true)
)

type LineMsg string

type IterMsg struct {
	Iteration int
	Total     int
	Title     string
	Role      string
}

type DoneMsg struct {
	Err error
}

type tickMsg time.Time

type Model struct {
	Cfg config.Config

	lines     []string
	viewport  viewport.Model
	autoOn    bool
	running   bool
	done      bool
	DoneErr   error
	iteration int
	total     int
	title     string
	role      string
	startTime time.Time
	elapsed   time.Duration
	ready     bool
	quitting  bool
	width     int
	height    int

	warningCount int
	nextAction   string

	lineChan   chan string
	iterChan   chan IterMsg
	doneChan   chan error
	startRunFn func(lineChan chan<- string, iterChan chan<- IterMsg, doneChan chan<- error)
}

func NewModel(cfg config.Config, maxIter int, startRun func(lineChan chan<- string, iterChan chan<- IterMsg, doneChan chan<- error)) Model {
	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().PaddingLeft(1)

	lineChan := make(chan string, 200)
	iterChan := make(chan IterMsg, 10)
	doneChan := make(chan error, 1)

	return Model{
		Cfg:        cfg,
		viewport:   vp,
		autoOn:     true,
		total:      maxIter,
		lines:      make([]string, 0, 100),
		lineChan:   lineChan,
		iterChan:   iterChan,
		doneChan:   doneChan,
		startRunFn: startRun,
	}
}

func (m Model) Init() tea.Cmd {
	go m.startRunFn(m.lineChan, m.iterChan, m.doneChan)
	return tea.Batch(m.tick(), m.listen())
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) listen() tea.Cmd {
	return func() tea.Msg {
		select {
		case line, ok := <-m.lineChan:
			if !ok {
				return doneWaitingMsg{}
			}
			return LineMsg(line)
		case iter, ok := <-m.iterChan:
			if !ok {
				return doneWaitingMsg{}
			}
			return iter
		case err := <-m.doneChan:
			return DoneMsg{Err: err}
		}
	}
}

func (m Model) waitDone() tea.Cmd {
	return func() tea.Msg {
		err := <-m.doneChan
		return DoneMsg{Err: err}
	}
}

type doneWaitingMsg struct{}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "s":
			m.autoOn = !m.autoOn
			if m.autoOn {
				m.viewport.GotoBottom()
			}
			return m, nil
		case "up", "down", "pgup", "pgdn", "halfpgup", "halfpgdn":
			m.autoOn = false
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		case "g", "home":
			m.autoOn = false
			m.viewport.GotoTop()
			return m, nil
		case "G", "end":
			m.autoOn = true
			m.viewport.GotoBottom()
			return m, nil
		default:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerH := m.headerHeight()
		footerH := m.footerHeight()
		m.viewport.Width = msg.Width - 2
		m.viewport.Height = msg.Height - headerH - footerH
		m.ready = true
		m.updateViewport()
		return m, nil

	case tickMsg:
		if m.running && !m.done && !m.startTime.IsZero() {
			m.elapsed = time.Since(m.startTime)
		}
		return m, m.tick()

	case LineMsg:
		m.lines = append(m.lines, string(msg))
		if len(m.lines) > maxLines {
			m.lines = m.lines[len(m.lines)-maxLines:]
		}
		if isGHWarning(string(msg)) {
			m.warningCount++
		}
		m.updateViewport()
		if m.autoOn {
			m.viewport.GotoBottom()
		}
		return m, m.listen()

	case IterMsg:
		m.iteration = msg.Iteration
		m.total = msg.Total
		m.title = msg.Title
		m.role = msg.Role
		m.running = true
		m.startTime = time.Now()
		m.elapsed = 0
		m.updateViewport()
		return m, m.listen()

	case DoneMsg:
		m.done = true
		m.DoneErr = msg.Err
		m.running = false
		m.updateViewport()
		return m, tea.Quit

	case doneWaitingMsg:
		return m, m.waitDone()
	}

	return m, nil
}

func (m Model) headerHeight() int {
	if !m.ready {
		return 1
	}
	if m.title != "" {
		return 4
	}
	return 1
}

func (m Model) footerHeight() int {
	return 1
}

func (m Model) updateViewport() {
	m.viewport.SetContent(m.content())
}

func (m Model) content() string {
	if len(m.lines) == 0 && !m.done {
		return "Waiting for agent output..."
	}
	return strings.Join(m.lines, "\n")
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	return m.headerView() + "\n" + m.viewport.View() + "\n" + m.statusView()
}

func (m Model) headerView() string {
	var b strings.Builder

	if m.done {
		b.WriteString(titleStyle.Render("loop run — complete"))
	} else if m.title != "" {
		b.WriteString(titleStyle.Render(fmt.Sprintf("loop run  %d/%d", m.iteration, m.total)))
	} else {
		b.WriteString(titleStyle.Render("loop run"))
	}
	b.WriteString("\n")

	if m.title != "" {
		b.WriteString(labelStyle.Render("Issue") + "  " + valueStyle.Render(m.title))
		b.WriteString("  ")
		b.WriteString(labelStyle.Render("Role") + "  " + roleStyle.Render(m.role))
		b.WriteString("  ")
		b.WriteString(labelStyle.Render("Elapsed") + "  " + timerStyle.Render(formatDuration(m.elapsed)))
		b.WriteString("\n")
	}

	if m.done && m.DoneErr != nil {
		b.WriteString(errorStyle.Render("Error: "+m.DoneErr.Error()) + "\n")
	} else if m.done {
		b.WriteString(successStyle.Render("All iterations complete") + "\n")
	}

	return headerStyle.Render(b.String())
}

func (m Model) statusText() string {
	if m.done {
		if m.DoneErr != nil {
			return "error"
		}
		return "complete"
	}
	if m.running {
		return "running"
	}
	return "idle"
}

func (m Model) statusView() string {
	var b strings.Builder

	statusStr := m.statusText()
	b.WriteString(fmt.Sprintf("[%s]", statusStr))

	if statusStr == "running" {
		b.WriteString(fmt.Sprintf("  %d/%d", m.iteration, m.total))
	}

	if m.warningCount > 0 {
		b.WriteString(fmt.Sprintf("  warnings: %d", m.warningCount))
	}

	if m.nextAction != "" {
		b.WriteString(fmt.Sprintf("  next: %s", m.nextAction))
	}

	autoLabel := "OFF"
	if m.autoOn {
		autoLabel = "ON"
	}

	lines := len(m.lines)
	lineLabel := fmt.Sprintf("Lines: %d", lines)
	if lines >= maxLines {
		lineLabel = fmt.Sprintf("Lines: %d+", maxLines)
	}

	b.WriteString(fmt.Sprintf("  %s  Auto-scroll: %s", lineLabel, autoLabel))

	return statusStyle.Render(
		fmt.Sprintf("%s  %s", b.String(), helpStyle.Render("[s] toggle  [^s] save  ↑/↓ scroll  q quit")),
	)
}

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
