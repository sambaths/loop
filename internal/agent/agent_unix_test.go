//go:build !windows

package agent

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSetProcessGroup(t *testing.T) {
	cmd := exec.Command("echo", "test")
	if cmd.SysProcAttr != nil {
		t.Fatal("expected nil SysProcAttr before setProcessGroup")
	}
	setProcessGroup(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("expected non-nil SysProcAttr after setProcessGroup")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Error("expected Setpgid to be true")
	}
}

func TestSetProcessGroupPreservesExistingAttr(t *testing.T) {
	cmd := exec.Command("echo", "test")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: false}
	setProcessGroup(cmd)
	if !cmd.SysProcAttr.Setpgid {
		t.Error("expected Setpgid to be set to true")
	}
}

func TestKillProcessGroupHandlesStartedCmd(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "sleep 60")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		RunAgentContext(ctx, "# Issue", "## Prompt", ".", 0)
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("agent subprocess was not killed within 3s of context cancellation")
	}
}

func TestKillProcessGroupStreamedHandlesCancel(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "for i in 1 2 3; do sleep 20; done")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		RunAgentContextStreamed(ctx, "# Issue", "## Prompt", ".", 0, 0, 0, func(line string) {})
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("streamed agent subprocess was not killed within 3s of context cancellation")
	}
}

func TestWatchdogWarnFires(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo "line1"; sleep 0.3; echo "__LOOP_RESULT__"; echo "COMPLETE"; echo "__LOOP_RESULT_END__"`)
	}

	stderrR, stderrW, _ := os.Pipe()
	origStderr := os.Stderr
	os.Stderr = stderrW

	var captured []string
	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0,
		100*time.Millisecond, 0,
		func(line string) {
			captured = append(captured, line)
		})

	stderrW.Close()
	os.Stderr = origStderr

	var stderrBuf bytes.Buffer
	io.Copy(&stderrBuf, stderrR)
	stderrR.Close()

	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE, got %q", result.Outcome)
	}
	if !strings.Contains(stderrBuf.String(), "watchdog:") {
		t.Errorf("expected watchdog warning in stderr, got:\n%s", stderrBuf.String())
	}
	if len(captured) == 0 {
		t.Error("expected at least one line of captured output")
	}
}

func TestWatchdogRecoverKillsAndRecovers(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	var mockCalls int
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		mockCalls++
		if mockCalls == 1 {
			return exec.Command("sh", "-c", "sleep 10")
		}
		return exec.Command("echo", "-n", "__LOOP_RESULT__\nCOMPLETE\n__LOOP_RESULT_END__")
	}

	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0,
		50*time.Millisecond, 200*time.Millisecond,
		func(line string) {})

	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE from recovery, got %q", result.Outcome)
	}
	if result.Err == nil {
		t.Error("expected non-nil error (process was killed)")
	}
	if mockCalls < 2 {
		t.Errorf("expected execCommandContext to be called at least twice (agent + recovery), got %d calls", mockCalls)
	}
}

func TestWatchdogDisabledWhenZero(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo "output"; echo "__LOOP_RESULT__"; echo "COMPLETE"; echo "__LOOP_RESULT_END__"`)
	}

	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0,
		0, 0,
		func(line string) {})

	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE, got %q", result.Outcome)
	}
}

func TestWatchdogNormalOutputResetsTimer(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c",
			`echo "line1"; sleep 0.15; echo "line2"; sleep 0.15; echo "line3"; echo "__LOOP_RESULT__"; echo "COMPLETE"; echo "__LOOP_RESULT_END__"`)
	}

	var captures int
	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0,
		200*time.Millisecond, 0,
		func(line string) {
			captures++
		})

	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE, got %q", result.Outcome)
	}
	if captures < 3 {
		t.Errorf("expected at least 3 lines captured, got %d", captures)
	}
}

func TestWatchdogRecoverFromInactivityRespectsInactivityRecoverDisabled(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo "line1"; sleep 0.3; echo "__LOOP_RESULT__"; echo "COMPLETE"; echo "__LOOP_RESULT_END__"`)
	}

	// Guard: recover disabled, warn enabled. Process completes before recover would fire.
	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0,
		50*time.Millisecond, 0,
		func(line string) {})

	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE, got %q", result.Outcome)
	}
}
