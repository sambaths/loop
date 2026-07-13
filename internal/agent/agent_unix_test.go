//go:build !windows

package agent

import (
	"context"
	"os/exec"
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
		RunAgentContextStreamed(ctx, "# Issue", "## Prompt", ".", 0, func(line string) {})
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
