package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sambaths/loop/internal/agent"
	"github.com/sambaths/loop/internal/config"
	"github.com/sambaths/loop/internal/git"
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

		issue.QuarantineAll(ps)

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

		// Derive temp branch name
		slug := strings.TrimSuffix(filepath.Base(selectedFile.FilePath), ".md")
		tempBranch := git.TempBranchName(slug)
		targetBranch := git.ResolveBranch(selectedFile.Branch, cfg.BranchOrigin)
		if targetBranch == "" {
			targetBranch = cfg.BranchOrigin
		}
		if targetBranch == "" {
			targetBranch = config.DefaultBranchOrigin
		}

		var promise agent.Promise
		var inactivityKill bool
		if iterErr := func() error {
			// Save user's working tree and note current branch
			stashed, stashErr := git.StashChanges()
			if stashErr != nil {
				return fmt.Errorf("stash before iteration: %w", stashErr)
			}
			origBranch, branchErr := git.CurrentBranch()
			if branchErr != nil {
				return fmt.Errorf("get current branch: %w", branchErr)
			}

			// Ensure we always switch back and restore stash, even on error/crash
			defer func() {
				if err := git.SwitchBranch(origBranch); err != nil {
					lineFn(fmt.Sprintf("--- warning: failed to switch back to %s: %v ---", origBranch, err))
				}
				if stashed {
					if applyErr := git.StashApply(); applyErr != nil {
						lineFn(fmt.Sprintf("--- warning: could not restore stash: %v (stash preserved) ---", applyErr))
					} else {
						if dropErr := git.StashDrop(); dropErr != nil {
							lineFn(fmt.Sprintf("--- warning: stash drop failed: %v ---", dropErr))
						}
					}
				}
			}()

			// Create/switch to temp branch
			if err := git.CreateTempBranch(tempBranch, targetBranch, cfg.BranchFromOrigin); err != nil {
				return fmt.Errorf("create temp branch: %w", err)
			}

			var runErr error
			promise, runErr = RunIterationStreamed(ctx, cfg, selectedFile, role, lineFn)
			if runErr != nil {
				if errors.Is(runErr, agent.ErrInactivityKill) {
					inactivityKill = true
					return nil
				}
				return fmt.Errorf("iteration %d: %w", i+1, runErr)
			}

			// On test pass, merge temp branch into target and delete it
			if role == issue.RoleTest && promise == agent.TestPass {
				if err := git.CleanWorkingTree(); err != nil {
					lineFn(fmt.Sprintf("--- warning: failed to clean working tree: %v ---", err))
				}

				if err := git.SwitchBranch(targetBranch); err != nil {
					lineFn(fmt.Sprintf("--- warning: failed to switch to %s for merge: %v ---", targetBranch, err))
				} else {
					if err := git.MergeBranch(tempBranch); err != nil {
						lineFn(fmt.Sprintf("--- warning: failed to merge %s into %s: %v ---", tempBranch, targetBranch, err))
						// Clean working tree and switch back to prevent state carry-over
						if cleanErr := git.CleanWorkingTree(); cleanErr != nil {
							lineFn(fmt.Sprintf("--- warning: failed to clean working tree after merge failure: %v ---", cleanErr))
						}
						if switchErr := git.SwitchBranch(origBranch); switchErr != nil {
							lineFn(fmt.Sprintf("--- warning: failed to switch back to %s: %v ---", origBranch, switchErr))
						}
					} else {
						if err := git.DeleteBranch(tempBranch); err != nil {
							lineFn(fmt.Sprintf("--- warning: failed to delete temp branch %s: %v ---", tempBranch, err))
						}
						lineFn(fmt.Sprintf("--- merged %s into %s ---", tempBranch, targetBranch))
						if err := git.SwitchBranch(origBranch); err != nil {
							lineFn(fmt.Sprintf("--- warning: failed to switch back to %s: %v ---", origBranch, err))
						}
					}
				}
			}

			return nil
		}(); iterErr != nil {
			return fmt.Errorf("iteration %d: %w", i+1, iterErr)
		}

		if inactivityKill {
			selectedFile.Retries++
			lineFn(fmt.Sprintf("--- issue %q killed by inactivity watchdog, recovery failed (retries: %d/%d) ---", selectedFile.Title, selectedFile.Retries, issue.MaxRetries))

			if selectedFile.Retries >= issue.MaxRetries {
				lineFn(fmt.Sprintf("--- issue %q exceeded max retries (%d) from inactivity kill, moving to unable/ ---", selectedFile.Title, selectedFile.Retries))
				if err := issue.SetRetryCount(selectedFile.FilePath, selectedFile.Retries); err != nil {
					lineFn(fmt.Sprintf("--- warning: failed to update retry count: %v ---", err))
				}
				if err := issue.Move(cfg.IssueDir, issueFromFile(selectedFile), issue.StateUnable); err != nil {
					lineFn(fmt.Sprintf("--- move to unable failed: %v ---", err))
				}
				continue
			}

			if err := issue.SetRetryCount(selectedFile.FilePath, selectedFile.Retries); err != nil {
				lineFn(fmt.Sprintf("--- warning: failed to update retry count: %v ---", err))
			}
			i--
			continue
		}

		if promise == agent.NoMoreTasks {
			lineFn("--- no more tasks — quarantining issue ---")
		}

		if freshPS, psErr := issue.ScanIssueDir(cfg.IssueDir); psErr == nil {
			if n, dupIssues := issue.QuarantineAll(freshPS); n > 0 {
				for _, di := range dupIssues {
					lineFn(fmt.Sprintf("%s: %s", di.Severity, di.Message))
				}
				lineFn(fmt.Sprintf("--- quarantined %d new duplicate(s) ---", n))
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

		target := issue.StateFromPath(filepath.Join(transition.DestDir, transition.Filename))

		if target == issue.StateTodo && selectedFile.State == issue.StateTestReady {
			if err := issue.StripSectionsFromFile(selectedFile.FilePath, []string{"Test Results", "UAT Results"}); err != nil {
				ghFailures = append(ghFailures, fmt.Sprintf("strip sections failed: %v", err))
			}

			selectedFile.Retries++
			if selectedFile.Retries >= issue.MaxRetries {
				target = issue.StateUnable
				lineFn(fmt.Sprintf("--- issue %q exceeded max retries (%d), moving to unable/ ---", selectedFile.Title, selectedFile.Retries))
			} else {
				if err := issue.SetRetryCount(selectedFile.FilePath, selectedFile.Retries); err != nil {
					lineFn(fmt.Sprintf("--- warning: failed to update retry count: %v ---", err))
				}
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

		issue.QuarantineAll(ps)

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

		// Derive temp branch name
		slug := strings.TrimSuffix(filepath.Base(selectedFile.FilePath), ".md")
		tempBranch := git.TempBranchName(slug)
		targetBranch := git.ResolveBranch(selectedFile.Branch, cfg.BranchOrigin)
		if targetBranch == "" {
			targetBranch = cfg.BranchOrigin
		}
		if targetBranch == "" {
			targetBranch = config.DefaultBranchOrigin
		}

		var promise agent.Promise
		var inactivityKill bool
		if iterErr := func() error {
			// Save user's working tree and note current branch
			stashed, stashErr := git.StashChanges()
			if stashErr != nil {
				return fmt.Errorf("stash before iteration: %w", stashErr)
			}
			origBranch, branchErr := git.CurrentBranch()
			if branchErr != nil {
				return fmt.Errorf("get current branch: %w", branchErr)
			}

			// Ensure we always switch back and restore stash, even on error/crash
			defer func() {
				if err := git.SwitchBranch(origBranch); err != nil {
					fmt.Fprintf(os.Stderr, "--- warning: failed to switch back to %s: %v ---\n", origBranch, err)
				}
				if stashed {
					if applyErr := git.StashApply(); applyErr != nil {
						fmt.Fprintf(os.Stderr, "--- warning: could not restore stash: %v (stash preserved) ---\n", applyErr)
					} else {
						if dropErr := git.StashDrop(); dropErr != nil {
							fmt.Fprintf(os.Stderr, "--- warning: stash drop failed: %v ---\n", dropErr)
						}
					}
				}
			}()

			// Create/switch to temp branch
			if err := git.CreateTempBranch(tempBranch, targetBranch, cfg.BranchFromOrigin); err != nil {
				return fmt.Errorf("create temp branch: %w", err)
			}

			var runErr error
			promise, runErr = RunIterationContext(ctx, cfg, selectedFile, role)
			if runErr != nil {
				if errors.Is(runErr, agent.ErrInactivityKill) {
					inactivityKill = true
					return nil
				}
				return fmt.Errorf("iteration %d: %w", i+1, runErr)
			}

			// On test pass, merge temp branch into target and delete it
			if role == issue.RoleTest && promise == agent.TestPass {
				if err := git.CleanWorkingTree(); err != nil {
					fmt.Fprintf(os.Stderr, "--- warning: failed to clean working tree: %v ---\n", err)
				}

				if err := git.SwitchBranch(targetBranch); err != nil {
					fmt.Fprintf(os.Stderr, "--- warning: failed to switch to %s for merge: %v ---\n", targetBranch, err)
				} else {
					if err := git.MergeBranch(tempBranch); err != nil {
						fmt.Fprintf(os.Stderr, "--- warning: failed to merge %s into %s: %v ---\n", tempBranch, targetBranch, err)
						// Clean working tree and switch back to prevent state carry-over
						if cleanErr := git.CleanWorkingTree(); cleanErr != nil {
							fmt.Fprintf(os.Stderr, "--- warning: failed to clean working tree after merge failure: %v ---\n", cleanErr)
						}
						if switchErr := git.SwitchBranch(origBranch); switchErr != nil {
							fmt.Fprintf(os.Stderr, "--- warning: failed to switch back to %s: %v ---\n", origBranch, switchErr)
						}
					} else {
						if err := git.DeleteBranch(tempBranch); err != nil {
							fmt.Fprintf(os.Stderr, "--- warning: failed to delete temp branch %s: %v ---\n", tempBranch, err)
						}
						fmt.Fprintf(os.Stderr, "--- merged %s into %s ---\n", tempBranch, targetBranch)
						if err := git.SwitchBranch(origBranch); err != nil {
							fmt.Fprintf(os.Stderr, "--- warning: failed to switch back to %s: %v ---\n", origBranch, err)
						}
					}
				}
			}

			return nil
		}(); iterErr != nil {
			return fmt.Errorf("iteration %d: %w", i+1, iterErr)
		}

		if inactivityKill {
			selectedFile.Retries++
			fmt.Fprintf(os.Stderr, "--- issue %q killed by inactivity watchdog, recovery failed (retries: %d/%d) ---\n", selectedFile.Title, selectedFile.Retries, issue.MaxRetries)

			if selectedFile.Retries >= issue.MaxRetries {
				fmt.Fprintf(os.Stderr, "--- issue %q exceeded max retries (%d) from inactivity kill, moving to unable/ ---\n", selectedFile.Title, selectedFile.Retries)
				if err := issue.SetRetryCount(selectedFile.FilePath, selectedFile.Retries); err != nil {
					fmt.Fprintf(os.Stderr, "--- warning: failed to update retry count: %v ---\n", err)
				}
				if err := issue.Move(cfg.IssueDir, issueFromFile(selectedFile), issue.StateUnable); err != nil {
					fmt.Fprintf(os.Stderr, "--- move to unable failed: %v ---\n", err)
				}
				continue
			}

			if err := issue.SetRetryCount(selectedFile.FilePath, selectedFile.Retries); err != nil {
				fmt.Fprintf(os.Stderr, "--- warning: failed to update retry count: %v ---\n", err)
			}
			i--
			continue
		}

		if promise == agent.NoMoreTasks {
			fmt.Fprintf(os.Stderr, "no more tasks — quarantining issue\n")
		}

		if freshPS, psErr := issue.ScanIssueDir(cfg.IssueDir); psErr == nil {
			if n, dupIssues := issue.QuarantineAll(freshPS); n > 0 {
				for _, di := range dupIssues {
					fmt.Fprintf(os.Stderr, "%s: %s\n", di.Severity, di.Message)
				}
				fmt.Fprintf(os.Stderr, "--- quarantined %d new duplicate(s) ---\n", n)
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

		target := issue.StateFromPath(filepath.Join(transition.DestDir, transition.Filename))

		if target == issue.StateTodo && selectedFile.State == issue.StateTestReady {
			if err := issue.StripSectionsFromFile(selectedFile.FilePath, []string{"Test Results", "UAT Results"}); err != nil {
				ghFailures = append(ghFailures, fmt.Sprintf("strip sections failed: %v", err))
			}

			selectedFile.Retries++
			if selectedFile.Retries >= issue.MaxRetries {
				target = issue.StateUnable
				fmt.Fprintf(os.Stderr, "--- issue %q exceeded max retries (%d), moving to unable/ ---\n", selectedFile.Title, selectedFile.Retries)
			} else {
				if err := issue.SetRetryCount(selectedFile.FilePath, selectedFile.Retries); err != nil {
					fmt.Fprintf(os.Stderr, "--- warning: failed to update retry count: %v ---\n", err)
				}
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
