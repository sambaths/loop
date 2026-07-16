package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sambaths/loop/internal/agent"
	"github.com/sambaths/loop/internal/config"
	"github.com/sambaths/loop/internal/git"
	"github.com/sambaths/loop/internal/issue"
	"github.com/sambaths/loop/internal/prompt"
)

func RunIteration(cfg *config.Config, issueFile *issue.IssueFile, role issue.Role) (agent.Promise, error) {
	return RunIterationContext(context.Background(), cfg, issueFile, role)
}

// RunIterationStreamed is like RunIterationContext but streams agent stdout
// lines to lineFn as they are produced.
func RunIterationStreamed(ctx context.Context, cfg *config.Config, issueFile *issue.IssueFile, role issue.Role, lineFn func(string)) (agent.Promise, error) {
	if issueFile.ExecMode == issue.ExecModeHITLOnly {
		lineFn(fmt.Sprintf("--- issue %q requires human-in-the-loop (HITL-only), skipping agent ---", issueFile.Title))
		return agent.NoMoreTasks, nil
	}

	timeout := time.Duration(cfg.AgentTimeout) * time.Second

	content, err := os.ReadFile(issueFile.FilePath)
	if err != nil {
		return "", fmt.Errorf("read issue file: %w", err)
	}

	body := fmt.Sprintf("## Role: %s\n\n", role)
	body += string(content)

	if role == issue.RoleImplement {
		body = issue.StripIssueSections(body, []string{"Test Results", "UAT Results"})
	}

	result, err := agent.RunAgentContextStreamed(ctx, body, prompt.GetPrompt(), cfg.IssueDir, timeout, lineFn)
	if err != nil {
		return "", fmt.Errorf("agent run: %w", err)
	}

	promise := resolvePromise(ctx, cfg.IssueDir, issueFile.Title, role, result)

	if role == issue.RoleImplement && *promise == agent.Complete {
		commitMsg := result.CommitMsg
		if commitMsg == "" ||
			strings.Contains(commitMsg, "<type>") ||
			strings.Contains(commitMsg, "<short summary>") ||
			strings.Contains(commitMsg, "login form") {
			commitType := inferCommitType(issueFile)
			commitMsg = fmt.Sprintf("%s: %s", commitType, issueFile.Title)
		}
		if err := git.CommitRaw(commitMsg); err != nil {
			return "", fmt.Errorf("commit changes: %w", err)
		}
	}

	return *promise, nil
}

func resolvePromise(ctx context.Context, issueDir, title string, role issue.Role, result *agent.Result) *agent.Promise {
	promise := agent.ParsePromises(result.Stdout.String())
	if promise != nil {
		return promise
	}

	stdout := result.Stdout.String()
	tailLen := len(stdout)
	if tailLen > 200 {
		tailLen = 200
	}
	if tailLen > 0 {
		fmt.Fprintf(os.Stderr, "warning: promise marker missing in agent output for issue %q (role %s); last %d bytes:\n%s\n", title, role, tailLen, stdout[len(stdout)-tailLen:])
	} else {
		fmt.Fprintf(os.Stderr, "warning: promise marker missing in agent output for issue %q (role %s); no output produced\n", title, role)
	}

	recovered := agent.RecoverPromise(ctx, issueDir, 30*time.Second)
	if recovered != nil {
		fmt.Fprintf(os.Stderr, "warning: promise recovery succeeded for issue %q, using recovered promise %q\n", title, *recovered)
		return recovered
	}

	fmt.Fprintf(os.Stderr, "warning: promise recovery failed for issue %q, defaulting to TEST_FAIL\n", title)
	defaultPromise := agent.TestFail
	return &defaultPromise
}

func RunIterationContext(ctx context.Context, cfg *config.Config, issueFile *issue.IssueFile, role issue.Role) (agent.Promise, error) {
	if issueFile.ExecMode == issue.ExecModeHITLOnly {
		fmt.Fprintf(os.Stderr, "--- issue %q requires human-in-the-loop (HITL-only), skipping agent ---\n", issueFile.Title)
		return agent.NoMoreTasks, nil
	}

	timeout := time.Duration(cfg.AgentTimeout) * time.Second

	content, err := os.ReadFile(issueFile.FilePath)
	if err != nil {
		return "", fmt.Errorf("read issue file: %w", err)
	}

	body := fmt.Sprintf("## Role: %s\n\n", role)
	body += string(content)

	if role == issue.RoleImplement {
		body = issue.StripIssueSections(body, []string{"Test Results", "UAT Results"})
	}

	result, err := agent.RunAgentContext(ctx, body, prompt.GetPrompt(), cfg.IssueDir, timeout)
	if err != nil {
		return "", fmt.Errorf("agent run: %w", err)
	}

	promise := resolvePromise(ctx, cfg.IssueDir, issueFile.Title, role, result)

	if role == issue.RoleImplement && *promise == agent.Complete {
		commitMsg := result.CommitMsg
		if commitMsg == "" ||
			strings.Contains(commitMsg, "<type>") ||
			strings.Contains(commitMsg, "<short summary>") ||
			strings.Contains(commitMsg, "login form") {
			commitType := inferCommitType(issueFile)
			commitMsg = fmt.Sprintf("%s: %s", commitType, issueFile.Title)
		}
		if err := git.CommitRaw(commitMsg); err != nil {
			return "", fmt.Errorf("commit changes: %w", err)
		}
	}

	return *promise, nil
}

func inferCommitType(f *issue.IssueFile) string {
	if f.Type != "" {
		return f.Type
	}
	title := strings.ToLower(f.Title)
	if strings.HasPrefix(title, "fix") || strings.HasPrefix(title, "bug") {
		return "bug"
	}
	if strings.HasPrefix(title, "add") || strings.HasPrefix(title, "implement") ||
		strings.HasPrefix(title, "feat") {
		return "feat"
	}
	return "enhancement"
}
