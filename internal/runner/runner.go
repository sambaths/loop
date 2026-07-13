package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sambaths/loop/internal/agent"
	"github.com/sambaths/loop/internal/config"
	"github.com/sambaths/loop/internal/github"
	"github.com/sambaths/loop/internal/issue"
	"github.com/sambaths/loop/internal/pipeline"
)

var ErrNoIssues = errors.New("no issues found in pipeline")

// ensureGHAuth checks that gh is authenticated when a repo is configured.
// If auth fails, it clears the repo config and falls back to local-only
// mode for this session. The warning is printed once per process by
// github.CheckAuthOnce.
func ensureGHAuth(cfg *config.Config) {
	if cfg.Repo == "" {
		return
	}
	if !github.CheckAuthOnce() {
		cfg.Repo = ""
	}
}

type Runner struct {
	Pipeline *pipeline.Pipeline
}

func New(p *pipeline.Pipeline) *Runner {
	return &Runner{Pipeline: p}
}

func (r *Runner) Run() error {
	for !r.Pipeline.Done() {
		err := r.Pipeline.Iterate()
		if err != nil {
			if errors.Is(err, issue.ErrNoIssues) {
				if r.Pipeline.Iteration == 0 {
					return ErrNoIssues
				}
				return nil
			}
			return fmt.Errorf("iteration %d: %w", r.Pipeline.Iteration, err)
		}
		fmt.Fprintf(os.Stderr, "completed iteration %d/%d\n", r.Pipeline.Iteration, r.Pipeline.MaxIterations)
	}
	return nil
}

func issueFromFile(f *issue.IssueFile) issue.Issue {
	return issue.Issue{
		FilePath:  f.FilePath,
		Title:     f.Title,
		State:     f.State,
		GitHubNum: f.GitHubNum,
	}
}

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

// RunLoopStreamed is like RunLoopContext but streams agent stdout lines to
// lineFn as they are produced. It also calls iterFn at the start of each
// iteration with the iteration number, total, issue title, and role.
func RunLoopStreamed(ctx context.Context, cfg *config.Config, maxIter int, forceIssueNum int, repair bool, lineFn func(string), iterFn func(iter, total int, title, role string)) error {
	ensureGHAuth(cfg)

	for i := 0; i < maxIter; i++ {
		ps, err := issue.ScanIssueDir(cfg.IssueDir)
		if err != nil {
			return fmt.Errorf("scan issue dir: %w", err)
		}

		issue.QuarantineDuplicates(ps)

		if pfIssues := issue.PreFlightCheck(ps, repair, cfg.ChecksumsEnabled); len(pfIssues) > 0 {
			hasErrors := false
			for _, pf := range pfIssues {
				lineFn(fmt.Sprintf("%s: %s", pf.Severity, pf.Message))
				if pf.Severity == issue.SeverityError {
					hasErrors = true
				}
			}
			if hasErrors {
				return issue.ErrPreFlightFailed
			}
		}

		var ghFailures []string

			if cfg.Repo != "" {
			repo, repoErr := github.RepoFromString(cfg.Repo)
			if repoErr == nil {
				for _, ci := range github.CheckClosedIssues(ps, repo) {
					lineFn(fmt.Sprintf("%s: %s", ci.Severity, ci.Message))
				}
				if repair {
					reopened, repairErr := github.RepairGitHubState(repo, ps)
					if repairErr != nil {
						ghFailures = append(ghFailures, fmt.Sprintf("repair GitHub state: %v", repairErr))
					}
					for _, ri := range reopened {
						lineFn(fmt.Sprintf("reopened prematurely closed GitHub issue #%d (%s)", ri.Number, ri.File))
					}
				}
				ensured, ensureErr := github.EnsureTestReadyLabels(repo, ps)
				if ensureErr != nil {
					ghFailures = append(ghFailures, fmt.Sprintf("ensure test-ready labels: %v", ensureErr))
				}
				for _, num := range ensured {
					lineFn(fmt.Sprintf("ensured test-ready label for GitHub issue #%d", num))
				}
			}
		}

		var selectedFile *issue.IssueFile
		var role issue.Role

		if i == 0 {
			if forceIssueNum > 0 {
				selectedFile, role, err = issue.FindIssueByNum(ps, forceIssueNum)
				if err != nil {
					if errors.Is(err, issue.ErrIssueNonAFK) {
						lineFn(fmt.Sprintf("WARNING: %v", err))
					}
					return fmt.Errorf("find issue #%d: %w", forceIssueNum, err)
				}
				if selectedFile == nil {
					return fmt.Errorf("issue #%d not found in pipeline", forceIssueNum)
				}
			} else {
				selectedFile, role, err = issue.SelectIssue(ps)
				if err != nil {
					if errors.Is(err, issue.ErrNoIssues) {
						lineFn("--- no issues found in pipeline ---")
						return nil
					}
					return fmt.Errorf("select issue: %w", err)
				}
			}
		} else {
			selectedFile, role, err = issue.SelectIssue(ps)
			if err != nil {
				if errors.Is(err, issue.ErrNoIssues) {
					lineFn("--- no issues found in pipeline ---")
					return nil
				}
				return fmt.Errorf("select issue: %w", err)
			}
		}

		iterFn(i+1, maxIter, selectedFile.Title, string(role))
		lineFn(fmt.Sprintf("--- iteration %d/%d: %s (%s) ---", i+1, maxIter, selectedFile.Title, role))

		promise, err := RunIterationStreamed(ctx, cfg, selectedFile, role, lineFn)
		if err != nil {
			return fmt.Errorf("iteration %d: %w", i+1, err)
		}

		if promise == agent.NoMoreTasks {
			lineFn("--- no more tasks — quarantining issue ---")
		}

		if freshPS, psErr := issue.ScanIssueDir(cfg.IssueDir); psErr == nil {
			if n, dupIssues := issue.QuarantineDuplicates(freshPS); n > 0 {
				for _, di := range dupIssues {
					lineFn(fmt.Sprintf("%s: %s", di.Severity, di.Message))
				}
				lineFn(fmt.Sprintf("--- quarantined %d new duplicate(s) ---", n))
			}
			if n, dupIssues := issue.QuarantineDuplicateGitHubNums(freshPS); n > 0 {
				for _, di := range dupIssues {
					lineFn(fmt.Sprintf("%s: %s", di.Severity, di.Message))
				}
				lineFn(fmt.Sprintf("--- quarantined %d duplicate GitHub issue(s) ---", n))
			}
			if n, dupIssues := issue.QuarantineDuplicateTitles(freshPS); n > 0 {
				for _, di := range dupIssues {
					lineFn(fmt.Sprintf("%s: %s", di.Severity, di.Message))
				}
				lineFn(fmt.Sprintf("--- quarantined %d duplicate title(s) ---", n))
			}
		}

		if _, statErr := os.Stat(selectedFile.FilePath); os.IsNotExist(statErr) {
			lineFn(fmt.Sprintf("--- selected issue %q was quarantined as duplicate ---", selectedFile.Title))
			continue
		}

		transition, err := issue.ComputeTransition(selectedFile, promise, role)
		if err != nil {
			lineFn(fmt.Sprintf("--- transition error: %v ---", err))
			if mvErr := issue.Move(cfg.IssueDir, issueFromFile(selectedFile), issue.StateQuarantine); mvErr != nil {
				lineFn(fmt.Sprintf("--- quarantine move failed: %v ---", mvErr))
			}
			continue
		}
		if transition == nil {
			continue
		}

		target := stateFromDir(transition.DestDir, cfg.IssueDir)

		if target == issue.StateTodo && selectedFile.State == issue.StateTestReady {
			if err := issue.StripSectionsFromFile(selectedFile.FilePath, []string{"Test Results", "UAT Results"}); err != nil {
				ghFailures = append(ghFailures, fmt.Sprintf("strip sections failed: %v", err))
			}
		}

		if err := issue.Move(cfg.IssueDir, issueFromFile(selectedFile), target); err != nil {
			lineFn(fmt.Sprintf("--- move to %s failed: %v ---", target, err))
			continue
		}

		lineFn(fmt.Sprintf("--- moved %s to %s ---", selectedFile.Title, target))

		if selectedFile.GitHubNum > 0 && cfg.Repo != "" {
			repo, repoErr := github.RepoFromString(cfg.Repo)
			if repoErr == nil {
				if err := github.SyncLabelsForStates(repo, selectedFile.GitHubNum, selectedFile.State, target); err != nil {
					ghFailures = append(ghFailures, fmt.Sprintf("github label sync for #%d: %v", selectedFile.GitHubNum, err))
				}
			}
		}

		if len(ghFailures) > 0 {
			lineFn(fmt.Sprintf("--- iteration %d github failures ---", i+1))
			for _, f := range ghFailures {
				lineFn(fmt.Sprintf("  gh failure: %s", f))
			}
			lineFn(fmt.Sprintf("--- end github failures ---"))
		}
	}
	return nil
}

// RunLoop drives the autonomous implement-then-test loop using a Config directly.
// It iterates up to maxIter times, scanning and selecting issues for each iteration.
// If forceIssueNum > 0, the first iteration targets that specific GitHub issue number;
// subsequent iterations use normal selection. Returns nil when no issues remain or
// the agent signals NO_MORE_TASKS.
func RunLoop(cfg *config.Config, maxIter int, forceIssueNum int) error {
	return RunLoopContext(context.Background(), cfg, maxIter, forceIssueNum, true)
}

// RunLoopContext is like RunLoop but uses the given context for cancellation.
// When the context is cancelled, the running agent iteration is killed and
// git context is restored before returning.
func RunLoopContext(ctx context.Context, cfg *config.Config, maxIter int, forceIssueNum int, repair bool) error {
	ensureGHAuth(cfg)

	for i := 0; i < maxIter; i++ {
		ps, err := issue.ScanIssueDir(cfg.IssueDir)
		if err != nil {
			return fmt.Errorf("scan issue dir: %w", err)
		}

		issue.QuarantineDuplicates(ps)

		if pfIssues := issue.PreFlightCheck(ps, repair, cfg.ChecksumsEnabled); len(pfIssues) > 0 {
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

			if cfg.Repo != "" {
			repo, repoErr := github.RepoFromString(cfg.Repo)
			if repoErr == nil {
				for _, ci := range github.CheckClosedIssues(ps, repo) {
					fmt.Fprintf(os.Stderr, "%s: %s\n", ci.Severity, ci.Message)
				}
				if repair {
					reopened, repairErr := github.RepairGitHubState(repo, ps)
					if repairErr != nil {
						ghFailures = append(ghFailures, fmt.Sprintf("repair GitHub state: %v", repairErr))
					}
					for _, ri := range reopened {
						fmt.Fprintf(os.Stderr, "reopened prematurely closed GitHub issue #%d (%s)\n", ri.Number, ri.File)
					}
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

		if i == 0 {
			if forceIssueNum > 0 {
				selectedFile, role, err = issue.FindIssueByNum(ps, forceIssueNum)
				if err != nil {
					if errors.Is(err, issue.ErrIssueNonAFK) {
						fmt.Fprintf(os.Stderr, "WARNING: %v\n", err)
					}
					return fmt.Errorf("find issue #%d: %w", forceIssueNum, err)
				}
				if selectedFile == nil {
					return fmt.Errorf("issue #%d not found in pipeline", forceIssueNum)
				}
			} else {
				selectedFile, role, err = issue.SelectIssue(ps)
				if err != nil {
					if errors.Is(err, issue.ErrNoIssues) {
						fmt.Fprintf(os.Stderr, "--- no issues found in pipeline ---\n")
						return nil
					}
					return fmt.Errorf("select issue: %w", err)
				}
			}
		} else {
			selectedFile, role, err = issue.SelectIssue(ps)
			if err != nil {
				if errors.Is(err, issue.ErrNoIssues) {
					fmt.Fprintf(os.Stderr, "--- no issues found in pipeline ---\n")
					return nil
				}
				return fmt.Errorf("select issue: %w", err)
			}
		}

		fmt.Fprintf(os.Stderr, "iteration %d/%d: %s (%s)\n", i+1, maxIter, selectedFile.Title, role)

		promise, err := RunIterationContext(ctx, cfg, selectedFile, role)
		if err != nil {
			return fmt.Errorf("iteration %d: %w", i+1, err)
		}

		if promise == agent.NoMoreTasks {
			fmt.Fprintf(os.Stderr, "no more tasks — quarantining issue\n")
		}

		if freshPS, psErr := issue.ScanIssueDir(cfg.IssueDir); psErr == nil {
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

		if _, statErr := os.Stat(selectedFile.FilePath); os.IsNotExist(statErr) {
			fmt.Fprintf(os.Stderr, "--- selected issue %q was quarantined as duplicate ---\n", selectedFile.Title)
			continue
		}

		transition, err := issue.ComputeTransition(selectedFile, promise, role)
		if err != nil {
			fmt.Fprintf(os.Stderr, "compute transition: %v\n", err)
			if mvErr := issue.Move(cfg.IssueDir, issueFromFile(selectedFile), issue.StateQuarantine); mvErr != nil {
				fmt.Fprintf(os.Stderr, "quarantine move after transition failure: %v\n", mvErr)
			}
			continue
		}
		if transition == nil {
			continue
		}

		target := stateFromDir(transition.DestDir, cfg.IssueDir)

		if target == issue.StateTodo && selectedFile.State == issue.StateTestReady {
			if err := issue.StripSectionsFromFile(selectedFile.FilePath, []string{"Test Results", "UAT Results"}); err != nil {
				ghFailures = append(ghFailures, fmt.Sprintf("strip sections failed: %v", err))
			}
		}

		if err := issue.Move(cfg.IssueDir, issueFromFile(selectedFile), target); err != nil {
			fmt.Fprintf(os.Stderr, "move to %s failed: %v\n", target, err)
			continue
		}

		if selectedFile.GitHubNum > 0 && cfg.Repo != "" {
			repo, repoErr := github.RepoFromString(cfg.Repo)
			if repoErr == nil {
				if err := github.SyncLabelsForStates(repo, selectedFile.GitHubNum, selectedFile.State, target); err != nil {
					ghFailures = append(ghFailures, fmt.Sprintf("github label sync for #%d: %v", selectedFile.GitHubNum, err))
				}
			}
		}

		if len(ghFailures) > 0 {
			fmt.Fprintf(os.Stderr, "--- iteration %d github failures ---\n", i+1)
			for _, f := range ghFailures {
				fmt.Fprintf(os.Stderr, "  gh failure: %s\n", f)
			}
			fmt.Fprintf(os.Stderr, "--- end github failures ---\n")
		}
	}
	return nil
}
