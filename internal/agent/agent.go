package agent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Promise is a valid promise marker token from an agent iteration.
type Promise string

const (
	Complete    Promise = "COMPLETE"
	TestPass    Promise = "TEST_PASS"
	TestFail    Promise = "TEST_FAIL"
	NoMoreTasks Promise = "NO_MORE_TASKS"
)

// Outcome represents the result of an agent iteration.
type Outcome string

const (
	OutcomeComplete    Outcome = "COMPLETE"
	OutcomeTestPass    Outcome = "TEST_PASS"
	OutcomeTestFail    Outcome = "TEST_FAIL"
	OutcomeNoMoreTasks Outcome = "NO_MORE_TASKS"
	OutcomeFail        Outcome = "FAIL"
)

var ErrOpencodeNotFound = errors.New("opencode binary not found in PATH")

func HasOpencode() bool {
	_, err := exec.LookPath("opencode")
	return err == nil
}

var (
	maxOutputSize  = 1 * 1024 * 1024 // 1 MB max stdout for promise parsing
	sentinelStart = "__LOOP_RESULT__"
	sentinelEnd   = "__LOOP_RESULT_END__"
)

// MaxOutputSize returns the maximum number of bytes of agent output to buffer
// for promise marker parsing. Output beyond this is discarded.
func MaxOutputSize() int { return maxOutputSize }

// Result holds the full outcome of an agent run.
type Result struct {
	Outcome   Outcome
	Output    bytes.Buffer // combined stdout + stderr (backward compat)
	Stdout    bytes.Buffer // stdout only (full output, even when truncated for parsing)
	Stderr    bytes.Buffer // stderr only (for logging)
	Err       error
	Truncated bool // true if stdout exceeded MaxOutputSize (parsed last N bytes)
}

// runContent runs opencode with the given content piped via stdin.
func runContent(ctx context.Context, content string, dir string, timeout time.Duration) (*Result, error) {
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	} else {
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	cmd := execCommandContext(ctx, "opencode", "run", "--dangerously-skip-permissions")
	setProcessGroup(cmd)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(content)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("opencode not found — install it at https://opencode.ai or check your PATH: %w", ErrOpencodeNotFound)
		}
		return nil, fmt.Errorf("start: %w", err)
	}

	go func() {
		<-ctx.Done()
		killProcessGroup(cmd)
	}()

	runErr := cmd.Wait()

	if runErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	truncated := false
	parseOutput := stdoutBuf.String()
	if len(parseOutput) > maxOutputSize {
		truncated = true
		fmt.Fprintf(os.Stderr, "warning: agent output (%d bytes) exceeds %d byte limit, parsing last %d bytes for promise marker\n", len(parseOutput), maxOutputSize, maxOutputSize)
		parseOutput = parseOutput[len(parseOutput)-maxOutputSize:]
	}

	outcome, found := ParseOutput(parseOutput)
	if !found {
		outcome = OutcomeFail
	}

	var combinedBuf bytes.Buffer
	combinedBuf.Write(stdoutBuf.Bytes())
	combinedBuf.Write(stderrBuf.Bytes())

	return &Result{
		Outcome:   outcome,
		Output:    combinedBuf,
		Stdout:    stdoutBuf,
		Stderr:    stderrBuf,
		Err:       runErr,
		Truncated: truncated,
	}, nil
}

// execCommand is overridable for testing.
var execCommand = exec.Command
var execCommandContext = exec.CommandContext

// Run executes opencode with the given prompt file piped via stdin.
func Run(dir string, promptFile string, timeout time.Duration) (*Result, error) {
	content, err := os.ReadFile(promptFile)
	if err != nil {
		return nil, fmt.Errorf("read prompt file: %w", err)
	}
	return runContent(context.Background(), string(content), dir, timeout)
}

// RunAgent runs opencode with the given issue text and prompt combined
// and piped via stdin. Composes content as issueText + "\n\n" + prompt
// (with empty prompt omitted) and runs with the given directory and timeout.
func RunAgent(issueText string, prompt string, dir string, timeout time.Duration) (*Result, error) {
	return RunAgentContext(context.Background(), issueText, prompt, dir, timeout)
}

// RunAgentContext is like RunAgent but uses the given context for cancellation.
// When the context is cancelled, the agent subprocess is killed and the error
// from the context is returned.
func RunAgentContext(ctx context.Context, issueText string, prompt string, dir string, timeout time.Duration) (*Result, error) {
	content := issueText
	if prompt != "" {
		content += "\n\n" + prompt
	}
	return runContent(ctx, content, dir, timeout)
}

// RunAgentContextStreamed is like RunAgentContext but streams stdout lines
// to lineFn as they are produced, while still buffering for promise parsing.
func RunAgentContextStreamed(ctx context.Context, issueText string, prompt string, dir string, timeout time.Duration, lineFn func(string)) (*Result, error) {
	content := issueText
	if prompt != "" {
		content += "\n\n" + prompt
	}
	return runContentStreamed(ctx, content, dir, timeout, lineFn)
}

// runContentStreamed runs opencode with streaming stdout output.
func runContentStreamed(ctx context.Context, content string, dir string, timeout time.Duration, lineFn func(string)) (*Result, error) {
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	} else {
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	cmd := execCommandContext(ctx, "opencode", "run", "--dangerously-skip-permissions")
	setProcessGroup(cmd)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(content)

	var stdoutBuf, stderrBuf bytes.Buffer
	streamReader, streamWriter := io.Pipe()
	cmd.Stdout = io.MultiWriter(&stdoutBuf, streamWriter)
	cmd.Stderr = &stderrBuf

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(streamReader)
		for scanner.Scan() {
			lineFn(scanner.Text())
		}
	}()

	if err := cmd.Start(); err != nil {
		streamWriter.Close()
		wg.Wait()
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("opencode not found — install it at https://opencode.ai or check your PATH: %w", ErrOpencodeNotFound)
		}
		return nil, fmt.Errorf("start: %w", err)
	}

	go func() {
		<-ctx.Done()
		killProcessGroup(cmd)
	}()

	runErr := cmd.Wait()
	streamWriter.Close()
	wg.Wait()

	if runErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	truncated := false
	parseOutput := stdoutBuf.String()
	if len(parseOutput) > maxOutputSize {
		truncated = true
		fmt.Fprintf(os.Stderr, "warning: agent output (%d bytes) exceeds %d byte limit, parsing last %d bytes for promise marker\n", len(parseOutput), maxOutputSize, maxOutputSize)
		parseOutput = parseOutput[len(parseOutput)-maxOutputSize:]
	}

	outcome, found := ParseOutput(parseOutput)
	if !found {
		outcome = OutcomeFail
	}

	var combinedBuf bytes.Buffer
	combinedBuf.Write(stdoutBuf.Bytes())
	combinedBuf.Write(stderrBuf.Bytes())

	return &Result{
		Outcome:   outcome,
		Output:    combinedBuf,
		Stdout:    stdoutBuf,
		Stderr:    stderrBuf,
		Err:       runErr,
		Truncated: truncated,
	}, nil
}

// ParsePromises extracts the last valid promise marker from agent output.
// Returns nil if no known promise marker is found.
func ParsePromises(stdout string) *Promise {
	outcome, found := ParseOutput(stdout)
	if !found {
		return nil
	}
	p := Promise(outcome)
	switch p {
	case Complete, TestPass, TestFail, NoMoreTasks:
		return &p
	default:
		return nil
	}
}

// ParseOutput extracts the last promise marker from agent output (bottom-up scan).
func ParseOutput(output string) (Outcome, bool) {
	startIdx := strings.LastIndex(output, sentinelStart)
	if startIdx == -1 {
		return "", false
	}
	startIdx += len(sentinelStart)

	remainder := output[startIdx:]
	endIdx := strings.Index(remainder, sentinelEnd)
	if endIdx == -1 {
		return "", false
	}

	token := strings.TrimSpace(remainder[:endIdx])

	switch token {
	case string(OutcomeComplete):
		return OutcomeComplete, true
	case string(OutcomeTestPass):
		return OutcomeTestPass, true
	case string(OutcomeTestFail):
		return OutcomeTestFail, true
	case string(OutcomeNoMoreTasks):
		return OutcomeNoMoreTasks, true
	default:
		return "", false
	}
}
