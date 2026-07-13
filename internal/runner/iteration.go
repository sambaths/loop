package runner

import (
	"context"
	"fmt"
	"os"
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

	restore, err := git.SaveContext()
	if err != nil {
		return "", fmt.Errorf("save git context: %w", err)
	}
	defer restore()

	branchOrigin := cfg.BranchOrigin
	if branchOrigin == "" {
		branchOrigin = config.DefaultBranchOrigin
	}

	if _, err := git.SwitchForIssue(issueFile.Branch, branchOrigin); err != nil {
		return "", fmt.Errorf("switch for issue: %w", err)
	}

	timeout := time.Duration(cfg.AgentTimeout) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(config.DefaultAgentTimeout) * time.Second
	}

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

	promise := agent.ParsePromises(result.Stdout.String())
	if promise == nil {
		return "", fmt.Errorf("no valid promise found in agent output")
	}

	return *promise, nil
}

func RunIterationContext(ctx context.Context, cfg *config.Config, issueFile *issue.IssueFile, role issue.Role) (agent.Promise, error) {
	if issueFile.ExecMode == issue.ExecModeHITLOnly {
		fmt.Fprintf(os.Stderr, "--- issue %q requires human-in-the-loop (HITL-only), skipping agent ---\n", issueFile.Title)
		return agent.NoMoreTasks, nil
	}

	restore, err := git.SaveContext()
	if err != nil {
		return "", fmt.Errorf("save git context: %w", err)
	}
	defer restore()

	branchOrigin := cfg.BranchOrigin
	if branchOrigin == "" {
		branchOrigin = config.DefaultBranchOrigin
	}

	if _, err := git.SwitchForIssue(issueFile.Branch, branchOrigin); err != nil {
		return "", fmt.Errorf("switch for issue: %w", err)
	}

	timeout := time.Duration(cfg.AgentTimeout) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(config.DefaultAgentTimeout) * time.Second
	}

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

	promise := agent.ParsePromises(result.Stdout.String())
	if promise == nil {
		return "", fmt.Errorf("no valid promise found in agent output")
	}

	return *promise, nil
}
