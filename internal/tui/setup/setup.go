package setup

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sambaths/loop/internal/config"
)

type step int

const (
	stepIssueDir step = iota
	stepRepo
	stepBranch
	stepConfirm
	stepDone
)

type model struct {
	step     step
	inputs   []textinput.Model
	cfg      config.Config
	err      error
	title    string
	errMsg   string
	warnings []string
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

func NewModel() tea.Model {
	inputs := make([]textinput.Model, 3)

	inputs[0] = textinput.New()
	inputs[0].Placeholder = config.DefaultIssueDir
	inputs[0].Prompt = "┃ "
	inputs[0].Focus()
	inputs[0].SetValue(config.DefaultIssueDir)

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "e.g. my-org/my-repo (or leave empty)"
	inputs[1].Prompt = "┃ "

	inputs[2] = textinput.New()
	inputs[2].Placeholder = config.DefaultBranchOrigin
	inputs[2].Prompt = "┃ "
	inputs[2].SetValue(config.DefaultBranchOrigin)

	return model{
		step:   stepIssueDir,
		inputs: inputs,
		title:  "loop setup",
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.step == stepDone {
		return m, tea.Quit
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			return m.advanceStep()
		case "esc":
			if m.step > stepIssueDir {
				m.step--
				m.errMsg = ""
				return m, nil
			}
			return m, tea.Quit
		}
	}

	cmds := make([]tea.Cmd, 0, 2)
	switch m.step {
	case stepIssueDir:
		var cmd tea.Cmd
		m.inputs[0], cmd = m.inputs[0].Update(msg)
		cmds = append(cmds, cmd)
	case stepRepo:
		var cmd tea.Cmd
		m.inputs[1], cmd = m.inputs[1].Update(msg)
		cmds = append(cmds, cmd)
	case stepBranch:
		var cmd tea.Cmd
		m.inputs[2], cmd = m.inputs[2].Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) advanceStep() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepIssueDir:
		val := m.inputs[0].Value()
		if val == "" {
			val = config.DefaultIssueDir
		}
		m.cfg.IssueDir = val
		m.inputs[0].Blur()
		m.inputs[1].Focus()
		m.step = stepRepo
		return m, textinput.Blink

	case stepRepo:
		m.cfg.Repo = strings.TrimSpace(m.inputs[1].Value())
		m.errMsg = ""
		if m.cfg.Repo != "" {
			if err := m.validateRepo(m.cfg.Repo); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
		}
		m.inputs[1].Blur()
		m.inputs[2].Focus()
		m.step = stepBranch
		return m, textinput.Blink

	case stepBranch:
		val := m.inputs[2].Value()
		if val == "" {
			val = config.DefaultBranchOrigin
		}
		m.cfg.BranchOrigin = val
		m.inputs[2].Blur()
		m.step = stepConfirm

	case stepConfirm:
		if err := m.initProject(); err != nil {
			m.err = err
			m.errMsg = fmt.Sprintf("Project init failed: %v", err)
			return m, nil
		}
		if err := config.Save(m.cfg); err != nil {
			m.err = err
			m.errMsg = fmt.Sprintf("Error saving config: %v", err)
			return m, nil
		}
		m.step = stepDone
	}

	return m, nil
}

func (m model) validateRepo(repo string) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) not found — install it from https://cli.github.com/")
	}
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("GitHub CLI (gh) is not authenticated — run 'gh auth login' to enable GitHub features")
	}
	cmd = exec.Command("gh", "repo", "view", repo)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Repository %q not found or not accessible — check the owner/name and try again", repo)
	}
	return nil
}

func (m model) initProject() error {
	_, err := config.FindProjectRoot()
	if err == nil {
		return nil
	}
	cmd := exec.Command("git", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}

func (m model) View() string {
	if m.step == stepDone {
		return m.successView()
	}

	switch m.step {
	case stepIssueDir:
		return m.renderInput(0, "Issue directory path", "Where should loop look for issue files?", helpStyle.Render("Enter to confirm · Esc/q to quit"))
	case stepRepo:
		return m.renderInput(1, "GitHub repository (optional)", "e.g. my-org/my-repo — leave empty for local-only mode", helpStyle.Render("Enter to confirm · Esc to go back · q to quit"))
	case stepBranch:
		return m.renderInput(2, "Default branch", "Default branch for issue implementation", helpStyle.Render("Enter to confirm · Esc to go back · q to quit"))
	case stepConfirm:
		return m.confirmView()
	}
	return ""
}

func (m model) successView() string {
	s := titleStyle.Render(m.title) + "\n\n"
	s += "Configuration saved!\n\n"
	s += fmt.Sprintf("  Issue directory:  %s\n", m.cfg.IssueDir)
	if m.cfg.Repo != "" {
		s += fmt.Sprintf("  GitHub repo:      %s\n", m.cfg.Repo)
	} else {
		s += "  GitHub repo:      (local-only mode)\n"
	}
	s += fmt.Sprintf("  Branch origin:    %s\n", m.cfg.BranchOrigin)
	path, err := config.ConfigPath()
	if err == nil {
		s += fmt.Sprintf("\n  Config file:      %s\n", path)
	}
	s += helpStyle.Render("\nNext steps:\n")
	s += helpStyle.Render("  Run 'loop completion' to set up bash completions\n")
	s += helpStyle.Render("  Ensure loop is in your PATH (e.g. add to ~/.bashrc or ~/.zshrc)\n")
	return s
}

func (m model) renderInput(idx int, label, description, help string) string {
	s := titleStyle.Render(m.title) + "\n\n"
	s += labelStyle.Render(label) + "\n"
	s += helpStyle.Render(description) + "\n\n"
	s += m.inputs[idx].View() + "\n\n"
	if m.errMsg != "" {
		s += errorStyle.Render(m.errMsg) + "\n\n"
	}
	s += help
	return s
}

func (m model) confirmView() string {
	s := titleStyle.Render(m.title) + "\n\n"
	s += "Ready to save configuration:\n\n"
	s += fmt.Sprintf("  Issue directory:  %s\n", m.cfg.IssueDir)
	if m.cfg.Repo != "" {
		s += fmt.Sprintf("  GitHub repo:      %s\n", m.cfg.Repo)
	} else {
		s += "  GitHub repo:      (local-only mode)\n"
	}
	s += fmt.Sprintf("  Branch origin:    %s\n", m.cfg.BranchOrigin)
	path, err := config.ConfigPath()
	if err == nil {
		s += fmt.Sprintf("  Config file:      %s\n", path)
	}
	for _, w := range m.warnings {
		s += "\n" + errorStyle.Render("⚠ " + w)
	}
	if m.errMsg != "" {
		s += "\n" + errorStyle.Render("✗ "+m.errMsg) + "\n"
	}
	s += "\n" + helpStyle.Render("Press Enter to save · Esc to go back · q to quit")
	return s
}


