package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sambaths/loop/internal/agent"
	"github.com/sambaths/loop/internal/git"
	"github.com/sambaths/loop/internal/github"
	"github.com/sambaths/loop/internal/issue"
)

const defaultAgentTimeout = 5 * time.Minute

type Pipeline struct {
	IssueDir         string
	MaxIterations    int
	Iteration        int
	ForceIssueNum    int
	CurrentIssue     *issue.Issue
	BranchOrigin     string
	AgentTimeout     time.Duration
	Repo             string
	ChecksumsEnabled bool
}

func New(issueDir string, maxIterations int) *Pipeline {
	return &Pipeline{
		IssueDir:      issueDir,
		MaxIterations: maxIterations,
		BranchOrigin:  "main",
		AgentTimeout:  defaultAgentTimeout,
	}
}

func (p *Pipeline) Done() bool {
	return p.Iteration >= p.MaxIterations
}

func (p *Pipeline) Iterate() error {
	if p.Done() {
		return fmt.Errorf("pipeline already completed %d iterations", p.MaxIterations)
	}

	ps, err := issue.ScanIssueDir(p.IssueDir)
	if err != nil {
		return fmt.Errorf("scan issue dir: %w", err)
	}

	if n, _ := issue.QuarantineDuplicates(ps); n > 0 {
		fmt.Fprintf(os.Stderr, "quarantined %d duplicate file(s)\n", n)
	}

	if pfIssues := issue.PreFlightCheck(ps, false, p.ChecksumsEnabled); len(pfIssues) > 0 {
		hasErrors := false
		for _, pf := range pfIssues {
			fmt.Fprintf(os.Stderr, "%s: %s\n", pf.Severity, pf.Message)
			if pf.Severity == issue.SeverityError {
				hasErrors = true
			}
		}
		if hasErrors {
			return issue.ErrPreFlightFailed
		}
	}

	var ghFailures []string

	if p.Repo != "" && !github.CheckAuthOnce() {
		p.Repo = ""
	}

		if p.Repo != "" {
		repo, repoErr := github.RepoFromString(p.Repo)
		if repoErr == nil {
			for _, ci := range github.CheckClosedIssues(ps, repo) {
				fmt.Fprintf(os.Stderr, "%s: %s\n", ci.Severity, ci.Message)
			}
			for _, ni := range github.CheckIssueExistence(ps, repo) {
				fmt.Fprintf(os.Stderr, "%s: %s\n", ni.Severity, ni.Message)
			}
			reopened, repairErr := github.RepairGitHubState(repo, ps)
			if repairErr != nil {
				ghFailures = append(ghFailures, fmt.Sprintf("repair GitHub state: %v", repairErr))
			}
			for _, ri := range reopened {
				fmt.Fprintf(os.Stderr, "reopened prematurely closed GitHub issue #%d (%s)\n", ri.Number, ri.File)
			}
			ensured, ensureErr := github.EnsureTestReadyLabels(repo, ps)
			if ensureErr != nil {
				ghFailures = append(ghFailures, fmt.Sprintf("ensure test-ready labels: %v", ensureErr))
			}
			for _, num := range ensured {
				fmt.Fprintf(os.Stderr, "ensured test-ready label for GitHub issue #%d\n", num)
			}
		}
	}

	var selectedFile *issue.IssueFile
	var role issue.Role

	if p.Iteration == 0 && p.ForceIssueNum > 0 {
		selectedFile, role, err = issue.FindIssueByNum(ps, p.ForceIssueNum)
		if err != nil {
			if errors.Is(err, issue.ErrIssueNonAFK) {
				fmt.Fprintf(os.Stderr, "WARNING: %v\n", err)
			}
			return err
		}
		if selectedFile == nil {
			return fmt.Errorf("issue #%d not found in pipeline", p.ForceIssueNum)
		}
	} else {
		selectedFile, role, err = issue.SelectIssue(ps)
		if err != nil {
			return err
		}
	}

	currentIssue, err := issue.Read(selectedFile.FilePath)
	if err != nil {
		return fmt.Errorf("read issue: %w", err)
	}

	p.CurrentIssue = currentIssue
	p.Iteration++

	if selectedFile.ExecMode == issue.ExecModeHITLOnly {
		fmt.Fprintf(os.Stderr, "--- issue %q requires human-in-the-loop (HITL-only), quarantining ---\n", selectedFile.Title)
		if err := issue.Move(p.IssueDir, *currentIssue, issue.StateQuarantine); err != nil {
			fmt.Fprintf(os.Stderr, "quarantine move for HITL-only issue: %v\n", err)
		}
		return nil
	}

	restore, err := git.SaveContext()
	if err != nil {
		return fmt.Errorf("save git context: %w", err)
	}

	if _, err := git.SwitchForIssue(selectedFile.Branch, p.BranchOrigin); err != nil {
		restore()
		return fmt.Errorf("switch for issue: %w", err)
	}

	result, agentErr := agent.Run(p.IssueDir, p.CurrentIssue.FilePath, p.AgentTimeout)

	restore()

		if freshPS, psErr := issue.ScanIssueDir(p.IssueDir); psErr == nil {
			if n, dupIssues := issue.QuarantineDuplicates(freshPS); n > 0 {
				for _, di := range dupIssues {
					fmt.Fprintf(os.Stderr, "%s: %s\n", di.Severity, di.Message)
				}
				fmt.Fprintf(os.Stderr, "--- quarantined %d new duplicate(s) ---\n", n)
			}
			if n, dupIssues := issue.QuarantineDuplicateGitHubNums(freshPS); n > 0 {
				for _, di := range dupIssues {
					fmt.Fprintf(os.Stderr, "%s: %s\n", di.Severity, di.Message)
				}
				fmt.Fprintf(os.Stderr, "--- quarantined %d duplicate GitHub issue(s) ---\n", n)
			}
			if n, dupIssues := issue.QuarantineDuplicateTitles(freshPS); n > 0 {
				for _, di := range dupIssues {
					fmt.Fprintf(os.Stderr, "%s: %s\n", di.Severity, di.Message)
				}
				fmt.Fprintf(os.Stderr, "--- quarantined %d duplicate title(s) ---\n", n)
			}
		}

	if _, statErr := os.Stat(p.CurrentIssue.FilePath); os.IsNotExist(statErr) {
		fmt.Fprintf(os.Stderr, "--- selected file %q was quarantined as duplicate ---\n", p.CurrentIssue.Title)
		return nil
	}

	if agentErr != nil {
		fmt.Fprintf(os.Stderr, "agent failed: %v\n", agentErr)
		if mvErr := issue.Move(p.IssueDir, *p.CurrentIssue, issue.StateQuarantine); mvErr != nil {
			fmt.Fprintf(os.Stderr, "quarantine move after agent failure: %v\n", mvErr)
		}
		return nil
	}

	if result.Err != nil {
		logDir := filepath.Join(p.IssueDir, ".loop")
		os.MkdirAll(logDir, 0755)
		logPath := filepath.Join(logDir, fmt.Sprintf("run-%03d.log", p.Iteration))
		logEntry := fmt.Sprintf("=== Run %d at %s ===\n%s\n=== End run %d ===\n",
			p.Iteration, time.Now().Format(time.RFC3339), result.Stderr.String(), p.Iteration)
		os.WriteFile(logPath, []byte(logEntry), 0644)
	}

	if result.Outcome == agent.OutcomeFail {
		if err := issue.Move(p.IssueDir, *p.CurrentIssue, issue.StateQuarantine); err != nil {
			fmt.Fprintf(os.Stderr, "move to quarantine: %v\n", err)
		}
		return nil
	}

	promise := agent.Promise(result.Outcome)
	transition, err := issue.ComputeTransition(selectedFile, promise, role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "compute transition failed: %v\n", err)
		if mvErr := issue.Move(p.IssueDir, *p.CurrentIssue, issue.StateQuarantine); mvErr != nil {
			fmt.Fprintf(os.Stderr, "quarantine move after transition failure: %v\n", mvErr)
		}
		return nil
	}
	if transition == nil {
		return nil
	}

	target := stateFromDir(transition.DestDir, p.IssueDir)

	// When TEST_FAIL returns an issue from test-ready to issues/ (todo),
	// strip test results and UAT results so they don't accumulate on retry.
	if target == issue.StateTodo && p.CurrentIssue.State == issue.StateTestReady {
		if err := issue.StripSectionsFromFile(p.CurrentIssue.FilePath, []string{"Test Results", "UAT Results"}); err != nil {
			fmt.Fprintf(os.Stderr, "strip sections failed: %v\n", err)
		}
	}

	if err := issue.Move(p.IssueDir, *p.CurrentIssue, target); err != nil {
		fmt.Fprintf(os.Stderr, "move to %s failed: %v\n", target, err)
		return nil
	}

	if p.CurrentIssue.GitHubNum > 0 && p.Repo != "" {
		repo, repoErr := github.RepoFromString(p.Repo)
		if repoErr == nil {
			fromState := p.CurrentIssue.State
			if err := github.SyncLabelsForStates(repo, p.CurrentIssue.GitHubNum, fromState, target); err != nil {
				ghFailures = append(ghFailures, fmt.Sprintf("github label sync for #%d: %v", p.CurrentIssue.GitHubNum, err))
			}
		}
	}

	if len(ghFailures) > 0 {
		fmt.Fprintf(os.Stderr, "--- iteration %d github failures ---\n", p.Iteration)
		for _, f := range ghFailures {
			fmt.Fprintf(os.Stderr, "  gh failure: %s\n", f)
		}
		fmt.Fprintf(os.Stderr, "--- end github failures ---\n")
	}

	return nil
}

// stateFromDir derives the issue State from a destination directory path.
// The todo directory is the issues root itself (no subdirectory), so any path
// that doesn't match a known state subdirectory resolves to StateTodo.
func stateFromDir(dir, issuesDir string) issue.State {
	base := filepath.Base(dir)
	if base == filepath.Base(issuesDir) {
		return issue.StateTodo
	}
	switch base {
	case "test-ready":
		return issue.StateTestReady
	case "ready-for-agent":
		return issue.StateReadyForAgent
	case "done":
		return issue.StateDone
	case ".quarantine":
		return issue.StateQuarantine
	default:
		return issue.StateTodo
	}
}
