package github

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sambaths/loop/internal/config"
	"github.com/sambaths/loop/internal/issue"
)

var ErrNotAvailable = errors.New("gh not available")
var ErrNoRepo = errors.New("no GitHub repo configured")
var ErrTransient = errors.New("transient gh error")

var (
	authOnce sync.Once
	authOK   bool
)

// CheckAuthOnce checks gh authentication once per process. Returns true if
// authenticated. On failure, prints a warning and returns false for all
// subsequent calls.
func CheckAuthOnce() bool {
	authOnce.Do(func() {
		err := AuthCheck()
		authOK = err == nil
		if !authOK {
			fmt.Fprintf(os.Stderr, "warning: GitHub CLI (gh) not authenticated — skipping all GitHub operations for this session\n")
		}
	})
	return authOK
}

// ResetAuthCheck clears the cached auth check result for use in tests.
func ResetAuthCheck() {
	authOnce = sync.Once{}
	authOK = false
}

func IsNotAvailable(err error) bool {
	return errors.Is(err, ErrNotAvailable)
}

func IsNoRepo(err error) bool {
	return errors.Is(err, ErrNoRepo)
}

func IsTransient(err error) bool {
	return errors.Is(err, ErrTransient)
}

func isTransientOutput(s string) bool {
	lower := strings.ToLower(s)
	patterns := []string{
		"connection refused",
		"i/o timeout",
		"dial tcp",
		"no such host",
		"could not resolve",
		"tls handshake",
		"network is unreachable",
		"client.timeout",
		"eof",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

type Repo struct {
	Owner string
	Name  string
}

func RepoFromString(s string) (Repo, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Repo{}, fmt.Errorf("invalid repo format %q, expected owner/name", s)
	}
	return Repo{Owner: parts[0], Name: parts[1]}, nil
}

func (r Repo) String() string {
	return r.Owner + "/" + r.Name
}

func stateLabel(state issue.State) string {
	switch state {
	case issue.StateReadyForAgent, issue.StateTodo:
		return "ready-for-agent"
	case issue.StateTestReady:
		return "test-ready"
	case issue.StateDone:
		return "done"
	case issue.StateQuarantine:
		return "quarantine"
	default:
		return ""
	}
}

func gh(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrNotAvailable
		}
		output := string(out)
		if isTransientOutput(output) || isTransientOutput(err.Error()) {
			return "", fmt.Errorf("gh %s: %w\noutput: %s", strings.Join(args, " "), ErrTransient, output)
		}
		return "", fmt.Errorf("gh %s: %w\noutput: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(out)), nil
}

func CheckInstalled() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("%w: gh not found in PATH", ErrNotAvailable)
	}
	return nil
}

func AuthCheck() error {
	_, err := gh("auth", "status")
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotAvailable, err.Error())
	}
	return nil
}

func RepoExists(r Repo) bool {
	_, err := gh("repo", "view", r.String())
	return err == nil
}

func IssueExists(r Repo, num int) bool {
	_, err := gh("issue", "view", fmt.Sprint(num), "--repo", r.String())
	return err == nil
}

func IssueState(r Repo, num int) (string, error) {
	state, err := gh("issue", "view", fmt.Sprint(num), "--repo", r.String(), "--json", "state", "--jq", ".state")
	if err != nil {
		return "", err
	}
	return state, nil
}

func IssueLabels(r Repo, num int) ([]string, error) {
	out, err := gh("issue", "view", fmt.Sprint(num), "--repo", r.String(), "--json", "labels", "--jq", "[.labels[].name] | join(\",\")")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, ","), nil
}

func AddLabel(r Repo, num int, label string) error {
	_, err := gh("issue", "edit", fmt.Sprint(num), "--repo", r.String(), "--add-label", label)
	return err
}

func RemoveLabel(r Repo, num int, label string) error {
	_, err := gh("issue", "edit", fmt.Sprint(num), "--repo", r.String(), "--remove-label", label)
	return err
}

func CloseIssue(r Repo, num int, comment string) error {
	args := []string{"issue", "close", fmt.Sprint(num), "--repo", r.String()}
	if comment != "" {
		args = append(args, "--comment", comment)
	}
	_, err := gh(args...)
	return err
}

func ReopenIssue(r Repo, num int) error {
	_, err := gh("issue", "reopen", fmt.Sprint(num), "--repo", r.String())
	return err
}

func CommentOnIssue(r Repo, num int, body string) error {
	_, err := gh("issue", "comment", fmt.Sprint(num), "--repo", r.String(), "--body", body)
	return err
}

func removeLabelIfExists(r Repo, num int, label string) error {
	err := RemoveLabel(r, num, label)
	if err != nil && (strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "label does not exist")) {
		return nil
	}
	return err
}

func SyncLabelsForStates(r Repo, issueNumber int, from, to issue.State) error {
	var firstErr error
	report := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	switch {
	case from == issue.StateTodo && to == issue.StateTestReady:
		report(removeLabelIfExists(r, issueNumber, "ready-for-agent"))
		report(removeLabelIfExists(r, issueNumber, "ready-for-human"))
		report(AddLabel(r, issueNumber, "test-ready"))
	case from == issue.StateTestReady && to == issue.StateDone:
		report(removeLabelIfExists(r, issueNumber, "test-ready"))
		report(CloseIssue(r, issueNumber, ""))
	case from == issue.StateTestReady && to == issue.StateTodo:
		report(removeLabelIfExists(r, issueNumber, "test-ready"))
		report(AddLabel(r, issueNumber, "ready-for-agent"))
	case from == issue.StateTestReady && to == issue.StateReadyForAgent:
		report(removeLabelIfExists(r, issueNumber, "test-ready"))
		report(AddLabel(r, issueNumber, "ready-for-agent"))
	case from == issue.StateReadyForAgent && to == issue.StateTestReady:
		report(removeLabelIfExists(r, issueNumber, "ready-for-agent"))
		report(AddLabel(r, issueNumber, "test-ready"))
	case from == issue.StateReadyForAgent && to == issue.StateDone:
		report(CloseIssue(r, issueNumber, ""))
	case from == issue.StateTodo && to == issue.StateDone:
		report(CloseIssue(r, issueNumber, ""))
	}
	return firstErr
}

// dirToState derives the issue state from a transition destination directory path.
func dirToState(dir string) issue.State {
	switch filepath.Base(dir) {
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

// SyncLabels syncs GitHub labels for an issue based on a pipeline transition.
// It loads the repository configuration and delegates to SyncLabelsForStates.
func SyncLabels(transition *issue.Transition, issueFile *issue.IssueFile) error {
	cfg, ok, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if !ok {
		return ErrNoRepo
	}
	if cfg.Repo == "" {
		return nil
	}

	repo, err := RepoFromString(cfg.Repo)
	if err != nil {
		return err
	}

	return SyncLabelsForStates(repo, issueFile.GitHubNum, issueFile.State, dirToState(transition.DestDir))
}

func ReopenIfClosed(r Repo, issueNumber int) (bool, error) {
	state, err := IssueState(r, issueNumber)
	if err != nil {
		return false, fmt.Errorf("check issue state: %w", err)
	}
	if strings.EqualFold(state, "CLOSED") {
		if err := ReopenIssue(r, issueNumber); err != nil {
			return false, fmt.Errorf("reopen issue: %w", err)
		}
		if err := CommentOnIssue(r, issueNumber, "This issue was closed while still pending in the pipeline. Reopened automatically by loop."); err != nil {
			return false, fmt.Errorf("comment on reopen: %w", err)
		}
		return true, nil
	}
	return false, nil
}

func SyncLabel(repo Repo, issueNumber int, state issue.State) error {
	label := stateLabel(state)
	if label == "" {
		return fmt.Errorf("unknown state: %s", state)
	}

	_, err := gh("issue", "edit", fmt.Sprint(issueNumber),
		"--add-label", label,
		"--repo", repo.String(),
	)
	return err
}

type ReopenedIssue struct {
	Number int
	File   string
}

func RepairGitHubState(r Repo, ps *issue.PipelineState) ([]ReopenedIssue, error) {
	allPaths := ps.TodoFiles
	allPaths = append(allPaths, ps.TestReadyFiles...)
	allPaths = append(allPaths, ps.ReadyForAgentFiles...)

	var reopened []ReopenedIssue
	for _, p := range allPaths {
		f, err := issue.ParseIssueFile(p)
		if err != nil {
			continue
		}
		if f.GitHubNum <= 0 {
			continue
		}
		ok, err := ReopenIfClosed(r, f.GitHubNum)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to check/reopen issue #%d: %v\n", f.GitHubNum, err)
			continue
		}
		if ok {
			reopened = append(reopened, ReopenedIssue{Number: f.GitHubNum, File: p})
		}
	}
	return reopened, nil
}

// FixMissingLabels scans all non-done pipeline issues and ensures each has
// the correct GitHub label for its current state. Returns the list of GitHub
// issue numbers whose labels were updated.
func FixMissingLabels(r Repo, ps *issue.PipelineState) ([]int, error) {
	allPaths := ps.TodoFiles
	allPaths = append(allPaths, ps.TestReadyFiles...)
	allPaths = append(allPaths, ps.ReadyForAgentFiles...)

	var fixed []int
	for _, p := range allPaths {
		f, err := issue.ParseIssueFile(p)
		if err != nil {
			continue
		}
		if f.GitHubNum <= 0 {
			continue
		}
		expected := stateLabel(f.State)
		if expected == "" {
			continue
		}
		currentLabels, err := IssueLabels(r, f.GitHubNum)
		if err != nil {
			if IsTransient(err) {
				fmt.Fprintf(os.Stderr, "warning: transient error reading labels for #%d: %v\n", f.GitHubNum, err)
			}
			continue
		}
		hasLabel := false
		for _, l := range currentLabels {
			if strings.EqualFold(l, expected) {
				hasLabel = true
				break
			}
		}
		if !hasLabel {
			if err := AddLabel(r, f.GitHubNum, expected); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to add label %q to issue #%d: %v\n", expected, f.GitHubNum, err)
				continue
			}
			fixed = append(fixed, f.GitHubNum)
		}
	}
	return fixed, nil
}

// EnsureTestReadyLabels checks each file in the test-ready pipeline state and,
// if the corresponding GitHub issue is missing the "test-ready" label, adds it.
// Returns the list of GitHub issue numbers whose labels were updated.
func EnsureTestReadyLabels(r Repo, ps *issue.PipelineState) ([]int, error) {
	var fixed []int
	for _, p := range ps.TestReadyFiles {
		f, err := issue.ParseIssueFile(p)
		if err != nil {
			continue
		}
		if f.GitHubNum <= 0 {
			continue
		}
		currentLabels, err := IssueLabels(r, f.GitHubNum)
		if err != nil {
			if IsTransient(err) {
				fmt.Fprintf(os.Stderr, "warning: transient error reading labels for #%d: %v\n", f.GitHubNum, err)
			}
			continue
		}
		hasLabel := false
		for _, l := range currentLabels {
			if strings.EqualFold(l, "test-ready") {
				hasLabel = true
				break
			}
		}
		if !hasLabel {
			if err := AddLabel(r, f.GitHubNum, "test-ready"); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to add test-ready label to issue #%d: %v\n", f.GitHubNum, err)
				continue
			}
			fixed = append(fixed, f.GitHubNum)
		}
	}
	return fixed, nil
}

// CheckClosedIssues returns PreFlightIssue entries for issues in the todo or
// test-ready state whose corresponding GitHub issue is closed. This is a
// warning-level check since RepairGitHubState will reopen them automatically.
func CheckClosedIssues(ps *issue.PipelineState, r Repo) []issue.PreFlightIssue {
	allPaths := ps.TodoFiles
	allPaths = append(allPaths, ps.TestReadyFiles...)

	var issues []issue.PreFlightIssue
	for _, p := range allPaths {
		f, err := issue.ParseIssueFile(p)
		if err != nil {
			continue
		}
		if f.GitHubNum <= 0 {
			continue
		}
		state, err := IssueState(r, f.GitHubNum)
		if err != nil {
			if IsTransient(err) {
				fmt.Fprintf(os.Stderr, "warning: transient error checking issue #%d state: %v\n", f.GitHubNum, err)
			}
			continue
		}
		if strings.EqualFold(state, "CLOSED") {
			issues = append(issues, issue.PreFlightIssue{
				FilePath: p,
				Severity: issue.SeverityWarning,
				Message:  fmt.Sprintf("GitHub issue #%d is closed but still in %s", f.GitHubNum, f.State),
			})
		}
	}
	return issues
}

// CheckIssueExistence checks that each issue file's referenced GitHub issue
// number actually exists on the remote repo. Skips files with GitHubNum <= 0
// and files that cannot be parsed. Transient errors (network failures) are
// logged to stderr and skipped; only definitive "not found" errors produce
// PreFlightIssue entries.
func CheckIssueExistence(ps *issue.PipelineState, r Repo) []issue.PreFlightIssue {
	allPaths := ps.TodoFiles
	allPaths = append(allPaths, ps.TestReadyFiles...)
	allPaths = append(allPaths, ps.ReadyForAgentFiles...)

	var issues []issue.PreFlightIssue
	for _, p := range allPaths {
		f, err := issue.ParseIssueFile(p)
		if err != nil {
			continue
		}
		if f.GitHubNum <= 0 {
			continue
		}

		_, err = gh("issue", "view", fmt.Sprint(f.GitHubNum), "--repo", r.String())
		if err != nil {
			if IsTransient(err) {
				fmt.Fprintf(os.Stderr, "warning: transient error checking issue #%d: %v\n", f.GitHubNum, err)
				continue
			}
			issues = append(issues, issue.PreFlightIssue{
				FilePath: p,
				Severity: issue.SeverityError,
				Message:  fmt.Sprintf("GitHub issue #%d referenced in %s does not exist", f.GitHubNum, filepath.Base(p)),
			})
		}
	}
	return issues
}

func CreateIssue(repo Repo, title, body string) (int, error) {
	out, err := gh("issue", "create",
		"--title", title,
		"--body", body,
		"--repo", repo.String(),
		"--label", "test-ready",
	)
	if err != nil {
		return 0, err
	}

	var number int
	if _, err := fmt.Sscanf(out, "https://github.com/"+repo.Owner+"/"+repo.Name+"/issues/%d", &number); err != nil {
		return 0, fmt.Errorf("parse issue URL %q: %w", out, err)
	}
	return number, nil
}
