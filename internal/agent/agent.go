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
var ErrInactivityKill = errors.New("agent killed due to stdout inactivity watchdog")

func HasOpencode() bool {
	_, err := exec.LookPath("opencode")
	return err == nil
}

var (
	maxOutputSize       = 1 * 1024 * 1024 // 1 MB max stdout for promise parsing
	sentinelStart       = "__LOOP_RESULT__"
	sentinelEnd         = "__LOOP_RESULT_END__"
	commitSentinelStart = "__LOOP_COMMIT__"
	commitSentinelEnd   = "__LOOP_COMMIT_END__"
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
	CommitMsg string // commit message extracted from __LOOP_COMMIT__ sentinels
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

	result := &Result{
		Outcome:   outcome,
		Output:    combinedBuf,
		Stdout:    stdoutBuf,
		Stderr:    stderrBuf,
		Err:       runErr,
		Truncated: truncated,
	}
	result.CommitMsg = ParseCommitMessage(stdoutBuf.String())
	return result, nil
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
// inactivityWarn and inactivityRecover control the stdout inactivity watchdog
// (0 = disabled for each).
func RunAgentContextStreamed(ctx context.Context, issueText string, prompt string, dir string, timeout time.Duration, inactivityWarn, inactivityRecover time.Duration, lineFn func(string)) (*Result, error) {
	content := issueText
	if prompt != "" {
		content += "\n\n" + prompt
	}
	return runContentStreamed(ctx, content, dir, timeout, inactivityWarn, inactivityRecover, lineFn)
}

// runContentStreamed runs opencode with streaming stdout output.
// inactivityWarn and inactivityRecover control the stdout inactivity watchdog
// (0 = disabled for each).
func runContentStreamed(ctx context.Context, content string, dir string, timeout time.Duration, inactivityWarn, inactivityRecover time.Duration, lineFn func(string)) (*Result, error) {
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
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	cmd.Stdout = io.MultiWriter(&stdoutBuf, stdoutWriter)
	cmd.Stderr = io.MultiWriter(&stderrBuf, stderrWriter)

	var (
		lastOutputMu   sync.Mutex
		lastOutputTime = time.Now()
	)
	var watchdogKilled bool

	if inactivityWarn > 0 || inactivityRecover > 0 {
		watchdogCtx, stopWatchdog := context.WithCancel(ctx)
		defer stopWatchdog()

		go func() {
			checkInterval := 5 * time.Second
			if inactivityRecover > 0 && inactivityRecover < checkInterval {
				checkInterval = inactivityRecover / 2
			}
			if inactivityWarn > 0 && inactivityWarn < checkInterval {
				checkInterval = inactivityWarn / 2
			}

			ticker := time.NewTicker(checkInterval)
			defer ticker.Stop()

			warned := false

			for {
				select {
				case <-ticker.C:
					lastOutputMu.Lock()
					elapsed := time.Since(lastOutputTime)
					lastOutputMu.Unlock()

					if inactivityRecover > 0 && elapsed >= inactivityRecover {
						fmt.Fprintf(os.Stderr, "[loop] watchdog: agent inactive for %v (recover threshold: %v), killing process group\n", elapsed.Round(time.Second), inactivityRecover.Round(time.Second))
						killProcessGroup(cmd)
						watchdogKilled = true
						stopWatchdog()
						return
					}

					if !warned && inactivityWarn > 0 && elapsed >= inactivityWarn {
						fmt.Fprintf(os.Stderr, "[loop] watchdog: agent appears stalled (no stdout output for %v, warn threshold: %v)\n", elapsed.Round(time.Second), inactivityWarn.Round(time.Second))
						warned = true
					}

				case <-watchdogCtx.Done():
					return
				}
			}
		}()
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutReader)
		for scanner.Scan() {
			lastOutputMu.Lock()
			lastOutputTime = time.Now()
			lastOutputMu.Unlock()
			lineFn(scanner.Text())
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrReader)
		for scanner.Scan() {
			lineFn(scanner.Text())
		}
	}()

	if err := cmd.Start(); err != nil {
		stdoutWriter.Close()
		stderrWriter.Close()
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
	stdoutWriter.Close()
	stderrWriter.Close()
	wg.Wait()

	if watchdogKilled {
		fmt.Fprintf(os.Stderr, "[loop] watchdog: attempting promise recovery after inactivity kill\n")
		promise := RecoverPromise(ctx, dir, recoverTimeout)

		var outcome Outcome
		if promise != nil {
			outcome = Outcome(*promise)
		} else {
			outcome = OutcomeFail
			fmt.Fprintf(os.Stderr, "[loop] watchdog: promise recovery failed after inactivity kill\n")
		}

		var combinedBuf bytes.Buffer
		combinedBuf.Write(stdoutBuf.Bytes())
		combinedBuf.Write(stderrBuf.Bytes())

		result := &Result{
			Outcome: outcome,
			Output:  combinedBuf,
			Stdout:  stdoutBuf,
			Stderr:  stderrBuf,
			Err:     runErr,
		}
		result.CommitMsg = ParseCommitMessage(stdoutBuf.String())

		if promise == nil {
			return result, ErrInactivityKill
		}
		return result, nil
	}

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

	result := &Result{
		Outcome:   outcome,
		Output:    combinedBuf,
		Stdout:    stdoutBuf,
		Stderr:    stderrBuf,
		Err:       runErr,
		Truncated: truncated,
	}
	result.CommitMsg = ParseCommitMessage(stdoutBuf.String())
	return result, nil
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

// RecoverPrompt is the minimal prompt sent to recover a missing promise marker.
const RecoverPrompt = "Your previous output was missing a promise marker. What was the outcome? Output exactly one of: COMPLETE, TEST_PASS, TEST_FAIL, NO_MORE_TASKS wrapped in __LOOP_RESULT__ / __LOOP_RESULT_END__"

const recoverTimeout = 30 * time.Second

// RecoverPromise tries to recover a missing promise marker by re-invoking
// the agent with a minimal recovery prompt. Returns nil if recovery fails.
func RecoverPromise(ctx context.Context, dir string, timeout time.Duration) *Promise {
	result, err := RunAgentContext(ctx, "", RecoverPrompt, dir, timeout)
	if err != nil {
		return nil
	}
	return ParsePromises(result.Stdout.String())
}

// ParseCommitMessage extracts a commit message from between __LOOP_COMMIT__ sentinels.
// Returns empty string if not found.
func ParseCommitMessage(output string) string {
	startIdx := strings.LastIndex(output, commitSentinelStart)
	if startIdx == -1 {
		return ""
	}
	remainder := output[startIdx+len(commitSentinelStart):]
	endIdx := strings.Index(remainder, commitSentinelEnd)
	if endIdx == -1 {
		return ""
	}
	msg := strings.TrimSpace(remainder[:endIdx])
	return msg
}
