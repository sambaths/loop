package dashboard

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sambaths/loop/internal/config"
	"github.com/sambaths/loop/internal/issue"
	"github.com/sambaths/loop/internal/tui/screenshot"
)

type page int

const (
	pageOverview page = iota
	pageTodo
	pageTestReady
	pageDone
	pageQuarantine
	pageCount
)

type Model struct {
	Version         string
	cfg             config.Config
	page            page
	todo            []issue.Issue
	testReady       []issue.Issue
	done            []issue.Issue
	quarantined     []issue.Issue
	warnings        []issue.UnparseableFile
	stuckTestReady       []string
	invalidExecModes     []string
	transitions          []issue.TransitionEvent
	quit            bool
	screenshotSaved string
}

type screenshotMsg string

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	countStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFF00"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	todoStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#6A5ACD"))
	testStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#FF8C00"))
	doneStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#00CC66"))
	emptyBarSt   = lipgloss.NewStyle().Background(lipgloss.Color("#444444"))
	progressSt   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00CC66"))
	barLabelSt   = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	savedOkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC66"))
)

func NewModel(cfg config.Config) Model {
	m := Model{
		cfg:  cfg,
		page: pageOverview,
	}
	m.todo, _ = issue.List(cfg.IssueDir, issue.StateTodo)
	m.testReady, _ = issue.List(cfg.IssueDir, issue.StateTestReady)
	m.done, _ = issue.List(cfg.IssueDir, issue.StateDone)
	m.quarantined, _ = issue.List(cfg.IssueDir, issue.StateQuarantine)
	m.warnings, _ = issue.ScanUnparseable(cfg.IssueDir)
	m.stuckTestReady, _ = issue.FindStuckTestReadyFiles(cfg.IssueDir)
	m.invalidExecModes, _ = issue.FindInvalidExecModes(cfg.IssueDir)
	m.transitions, _ = issue.ReadTransitionLog(cfg.IssueDir, 5)
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		case "tab", "right", "l", "n":
			if m.page < pageCount-1 {
				m.page++
			}
		case "shift+tab", "left", "h", "p":
			if m.page > 0 {
				m.page--
			}
		case "s":
			name, err := screenshot.Save(m.View(), "dashboard")
			m.screenshotSaved = ""
			if err != nil {
				m.screenshotSaved = fmt.Sprintf("screenshot error: %v", err)
			} else {
				m.screenshotSaved = fmt.Sprintf("screenshot saved: %s", name)
			}
			return m, clearScreenshotCmd()
		}
	case screenshotMsg:
		m.screenshotSaved = ""
		return m, nil
	}
	return m, nil
}

func clearScreenshotCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return screenshotMsg("")
	})
}

func (m Model) headerView() string {
	version := m.Version
	if version == "" {
		version = "dev"
	}
	repo := m.cfg.Repo
	if repo == "" {
		repo = "local"
	}

	done := len(m.done)
	total := len(m.todo) + len(m.testReady) + len(m.done)
	iter := fmt.Sprintf("%d/%d", done, total)
	if total == 0 {
		iter = "-"
	}

	return headerStyle.Render(fmt.Sprintf("loop v%s  ·  %s  ·  %s", version, repo, iter))
}

func (m Model) View() string {
	if m.quit {
		return ""
	}
	s := m.headerView() + "\n\n"
	s += m.pageView() + "\n"
	if m.screenshotSaved != "" {
		s += savedOkStyle.Render(m.screenshotSaved) + "\n"
	}
	s += helpStyle.Render(fmt.Sprintf("Page %d/%d · tab/arrows to navigate · s screenshot · q to quit", int(m.page)+1, int(pageCount)))
	return s
}

func (m Model) pageView() string {
	switch m.page {
	case pageOverview:
		return m.overviewView()
	case pageTodo:
		return m.issueList("todo", m.todo)
	case pageTestReady:
		return m.issueList("test-ready", m.testReady)
	case pageDone:
		return m.issueList("done", m.done)
	case pageQuarantine:
		return m.issueList("quarantined", m.quarantined)
	default:
		return ""
	}
}

func (m Model) overviewView() string {
	s := "Pipeline overview\n\n"
	s += m.pipelineBar()
	if len(m.warnings) > 0 {
		s += "\n"
		for _, w := range m.warnings {
			s += warnStyle.Render(fmt.Sprintf("  %s (unparseable)\n", filepath.Base(w.Path)))
		}
	}
	if len(m.stuckTestReady) > 0 {
		s += "\n"
		for _, p := range m.stuckTestReady {
			s += warnStyle.Render(fmt.Sprintf("  %s (stuck in test-ready — has UAT Results but never transitioned)\n", filepath.Base(p)))
		}
	}
	if len(m.invalidExecModes) > 0 {
		s += "\n"
		for _, p := range m.invalidExecModes {
			s += warnStyle.Render(fmt.Sprintf("  %s (invalid Execution mode — review and fix manually)\n", filepath.Base(p)))
		}
	}
	if m.cfg.Repo != "" {
		s += infoStyle.Render(fmt.Sprintf("  repo:        %s\n", m.cfg.Repo))
	}
	s += infoStyle.Render(fmt.Sprintf("  issues dir:  %s\n", m.cfg.IssueDir))
	if len(m.transitions) > 0 {
		s += "\nRecent transitions\n"
		for _, t := range m.transitions {
			s += fmt.Sprintf("  %s  %q  %s → %s\n", t.Time.Format("15:04:05"), t.Title, t.From, t.To)
		}
	}
	return s
}

func (m Model) pipelineBar() string {
	todo := len(m.todo)
	testReady := len(m.testReady)
	done := len(m.done)
	quarantined := len(m.quarantined)
	total := todo + testReady + done

	if total+quarantined == 0 {
		return "  no issues found\n"
	}

	const barWidth = 40

	var bar strings.Builder
	bar.WriteString("  ")
	if total > 0 {
		todoW := int(math.Round(float64(todo) / float64(total) * barWidth))
		testW := int(math.Round(float64(testReady) / float64(total) * barWidth))
		doneW := barWidth - todoW - testW

		if todoW > 0 {
			bar.WriteString(todoStyle.Render(strings.Repeat(" ", todoW)))
		}
		if testW > 0 {
			bar.WriteString(testStyle.Render(strings.Repeat(" ", testW)))
		}
		if doneW > 0 {
			bar.WriteString(doneStyle.Render(strings.Repeat(" ", doneW)))
		}
	} else {
		bar.WriteString(emptyBarSt.Render(strings.Repeat(" ", barWidth)))
	}
	bar.WriteString("\n")

	var pct string
	if total > 0 {
		pct = progressSt.Render(fmt.Sprintf("%.0f%%", float64(done)/float64(total)*100))
	} else {
		pct = progressSt.Render("–")
	}

	bar.WriteString(fmt.Sprintf("  %s todo  %s test-ready  %s done  %s\n",
		barLabelSt.Render(fmt.Sprintf("%d", todo)),
		barLabelSt.Render(fmt.Sprintf("%d", testReady)),
		barLabelSt.Render(fmt.Sprintf("%d", done)),
		pct,
	))

	if quarantined > 0 {
		bar.WriteString(fmt.Sprintf("  quarantined: %s\n",
			barLabelSt.Render(fmt.Sprintf("%d", quarantined)),
		))
	}

	return bar.String()
}

func (m Model) issueList(name string, issues []issue.Issue) string {
	s := fmt.Sprintf("Issues (%s)\n\n", name)
	if len(issues) == 0 {
		s += "  (empty)"
	} else {
		for i, iss := range issues {
			gh := ""
			if iss.GitHubNum > 0 {
				gh = fmt.Sprintf(" (#%d)", iss.GitHubNum)
			}
			s += fmt.Sprintf("  %d. [%s] %s%s\n", i+1, iss.State, iss.Title, gh)
		}
	}
	return s
}
