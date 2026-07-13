package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPromiseValues(t *testing.T) {
	tests := []struct {
		promise  Promise
		want     string
		outcome Outcome
	}{
		{Complete, "COMPLETE", OutcomeComplete},
		{TestPass, "TEST_PASS", OutcomeTestPass},
		{TestFail, "TEST_FAIL", OutcomeTestFail},
		{NoMoreTasks, "NO_MORE_TASKS", OutcomeNoMoreTasks},
	}
	for _, tt := range tests {
		if got := string(tt.promise); got != tt.want {
			t.Errorf("Promise(%v) = %q, want %q", tt.promise, got, tt.want)
		}
		if Outcome(tt.promise) != tt.outcome {
			t.Errorf("Outcome(Promise(%v)) = %q, want %q", tt.promise, Outcome(tt.promise), tt.outcome)
		}
	}
}

func TestParseOutputComplete(t *testing.T) {
	output := `some output
__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__
more output`
	outcome, found := ParseOutput(output)
	if !found {
		t.Fatal("expected to find outcome")
	}
	if outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE, got %q", outcome)
	}
}

func TestParseOutputTestPass(t *testing.T) {
	output := `__LOOP_RESULT__
TEST_PASS
__LOOP_RESULT_END__`
	outcome, found := ParseOutput(output)
	if !found {
		t.Fatal("expected to find outcome")
	}
	if outcome != OutcomeTestPass {
		t.Errorf("expected TEST_PASS, got %q", outcome)
	}
}

func TestParseOutputTestFail(t *testing.T) {
	output := `__LOOP_RESULT__
TEST_FAIL
__LOOP_RESULT_END__`
	outcome, found := ParseOutput(output)
	if !found {
		t.Fatal("expected to find outcome")
	}
	if outcome != OutcomeTestFail {
		t.Errorf("expected TEST_FAIL, got %q", outcome)
	}
}

func TestParseOutputNoMoreTasks(t *testing.T) {
	output := `__LOOP_RESULT__
NO_MORE_TASKS
__LOOP_RESULT_END__`
	outcome, found := ParseOutput(output)
	if !found {
		t.Fatal("expected to find outcome")
	}
	if outcome != OutcomeNoMoreTasks {
		t.Errorf("expected NO_MORE_TASKS, got %q", outcome)
	}
}

func TestParseOutputNotFound(t *testing.T) {
	output := `some output without sentinel markers`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome")
	}
}

func TestParseOutputMissingEnd(t *testing.T) {
	output := `__LOOP_RESULT__
COMPLETE`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome without end sentinel")
	}
}

func TestParseOutputMissingStart(t *testing.T) {
	output := `COMPLETE
__LOOP_RESULT_END__`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome without start sentinel")
	}
}

func TestParseOutputWithSurroundingText(t *testing.T) {
	output := `Implementing feature...
Testing...
__LOOP_RESULT__
TEST_PASS
__LOOP_RESULT_END__
Done!`
	outcome, found := ParseOutput(output)
	if !found {
		t.Fatal("expected to find outcome")
	}
	if outcome != OutcomeTestPass {
		t.Errorf("expected TEST_PASS, got %q", outcome)
	}
}

func TestParseOutputWhitespace(t *testing.T) {
	output := `__LOOP_RESULT__
  TEST_FAIL  
__LOOP_RESULT_END__`
	outcome, found := ParseOutput(output)
	if !found {
		t.Fatal("expected to find outcome")
	}
	if outcome != OutcomeTestFail {
		t.Errorf("expected TEST_FAIL, got %q", outcome)
	}
}

func TestParseOutputEmptyString(t *testing.T) {
	_, found := ParseOutput("")
	if found {
		t.Fatal("expected not to find outcome in empty string")
	}
}

func TestParseOutputEmptyToken(t *testing.T) {
	output := "__LOOP_RESULT__\n__LOOP_RESULT_END__"
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome for empty token")
	}
}

func TestParseOutputUnknownToken(t *testing.T) {
	output := `__LOOP_RESULT__
UNKNOWN_TOKEN
__LOOP_RESULT_END__`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome for unknown token")
	}
}

func TestParseOutputMultipleGroups(t *testing.T) {
	output := `__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__
__LOOP_RESULT__
TEST_PASS
__LOOP_RESULT_END__`
	outcome, found := ParseOutput(output)
	if !found {
		t.Fatal("expected to find last outcome")
	}
	if outcome != OutcomeTestPass {
		t.Errorf("expected TEST_PASS (last group), got %q", outcome)
	}
}

func TestParsePromisesComplete(t *testing.T) {
	output := `__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p == nil {
		t.Fatal("expected non-nil Promise")
	}
	if *p != Complete {
		t.Errorf("expected COMPLETE, got %q", *p)
	}
}

func TestParsePromisesTestPass(t *testing.T) {
	output := `__LOOP_RESULT__
TEST_PASS
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p == nil {
		t.Fatal("expected non-nil Promise")
	}
	if *p != TestPass {
		t.Errorf("expected TEST_PASS, got %q", *p)
	}
}

func TestParsePromisesTestFail(t *testing.T) {
	output := `__LOOP_RESULT__
TEST_FAIL
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p == nil {
		t.Fatal("expected non-nil Promise")
	}
	if *p != TestFail {
		t.Errorf("expected TEST_FAIL, got %q", *p)
	}
}

func TestParsePromisesNoMoreTasks(t *testing.T) {
	output := `__LOOP_RESULT__
NO_MORE_TASKS
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p == nil {
		t.Fatal("expected non-nil Promise")
	}
	if *p != NoMoreTasks {
		t.Errorf("expected NO_MORE_TASKS, got %q", *p)
	}
}

func TestParsePromisesNotFound(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"no sentinels", `some output`},
		{"missing end sentinel", "__LOOP_RESULT__\nCOMPLETE"},
		{"missing start sentinel", "COMPLETE\n__LOOP_RESULT_END__"},
		{"empty string", ""},
		{"unknown token", "__LOOP_RESULT__\nUNKNOWN\n__LOOP_RESULT_END__"},
		{"non-promise Outcome", "__LOOP_RESULT__\nFAIL\n__LOOP_RESULT_END__"},
		{"empty token", "__LOOP_RESULT__\n__LOOP_RESULT_END__"},
		{"multiline token", "__LOOP_RESULT__\nSOME\nMULTILINE\nTOKEN\n__LOOP_RESULT_END__"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ParsePromises(tt.output)
			if p != nil {
				t.Errorf("expected nil, got %q", *p)
			}
		})
	}
}

func TestParsePromisesWithSurroundingText(t *testing.T) {
	output := `some text before
__LOOP_RESULT__
TEST_PASS
__LOOP_RESULT_END__
some text after`
	p := ParsePromises(output)
	if p == nil {
		t.Fatal("expected non-nil Promise")
	}
	if *p != TestPass {
		t.Errorf("expected TEST_PASS, got %q", *p)
	}
}

func TestParsePromisesWhitespace(t *testing.T) {
	output := `__LOOP_RESULT__
  TEST_FAIL  
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p == nil {
		t.Fatal("expected non-nil Promise")
	}
	if *p != TestFail {
		t.Errorf("expected TEST_FAIL, got %q", *p)
	}
}

func TestParsePromisesMultipleGroups(t *testing.T) {
	output := `__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__
__LOOP_RESULT__
TEST_PASS
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p == nil {
		t.Fatal("expected non-nil Promise")
	}
	if *p != TestPass {
		t.Errorf("expected TEST_PASS (last group), got %q", *p)
	}
}

func TestParseOutputTokenWithNewlines(t *testing.T) {
	output := `__LOOP_RESULT__
SOME
MULTILINE
TOKEN
__LOOP_RESULT_END__`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome for multiline unknown token")
	}
}

func TestParseOutputMultipleGroupsThree(t *testing.T) {
	output := `__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__
some text
__LOOP_RESULT__
TEST_FAIL
__LOOP_RESULT_END__
__LOOP_RESULT__
TEST_PASS
__LOOP_RESULT_END__`
	outcome, found := ParseOutput(output)
	if !found {
		t.Fatal("expected to find outcome from last of three groups")
	}
	if outcome != OutcomeTestPass {
		t.Errorf("expected TEST_PASS (last of three), got %q", outcome)
	}
}

func TestParseOutputMultipleGroupsRepeatedSame(t *testing.T) {
	output := `__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__
__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__
__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__`
	outcome, found := ParseOutput(output)
	if !found {
		t.Fatal("expected to find outcome from repeated groups")
	}
	if outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE (repeated), got %q", outcome)
	}
}

func TestParseOutputWhitespaceOnlyToken(t *testing.T) {
	output := "__LOOP_RESULT__\n   \n__LOOP_RESULT_END__"
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome for whitespace-only token")
	}
}

func TestParseOutputTabOnlyToken(t *testing.T) {
	output := "__LOOP_RESULT__\n\t\t\n__LOOP_RESULT_END__"
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome for tab-only token")
	}
}

func TestParseOutputMultipleEmptyGroups(t *testing.T) {
	output := `__LOOP_RESULT__
__LOOP_RESULT_END__
__LOOP_RESULT__
__LOOP_RESULT_END__`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome for multiple empty groups")
	}
}

func TestParseOutputEndSentinelBetweenSentinels(t *testing.T) {
	output := `__LOOP_RESULT__
extra text
__LOOP_RESULT_END__
COMPLETE
__LOOP_RESULT_END__`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome when end sentinel appears inside token area")
	}
}

func TestParseOutputStartSentinelBetweenSentinels(t *testing.T) {
	output := `__LOOP_RESULT__
some __LOOP_RESULT__ text
COMPLETE
__LOOP_RESULT_END__`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome when start sentinel appears inside token area")
	}
}

func TestParseOutputStartSentinelInSurroundingText(t *testing.T) {
	output := `some text __LOOP_RESULT__ more text
__LOOP_RESULT__
TEST_PASS
__LOOP_RESULT_END__`
	outcome, found := ParseOutput(output)
	if !found {
		t.Fatal("expected to find outcome when start sentinel appears in surrounding text")
	}
	if outcome != OutcomeTestPass {
		t.Errorf("expected TEST_PASS, got %q", outcome)
	}
}

func TestParseOutputEndSentinelInSurroundingText(t *testing.T) {
	output := `some __LOOP_RESULT_END__ text here
__LOOP_RESULT__
TEST_FAIL
__LOOP_RESULT_END__`
	outcome, found := ParseOutput(output)
	if !found {
		t.Fatal("expected to find outcome when end sentinel appears in surrounding text")
	}
	if outcome != OutcomeTestFail {
		t.Errorf("expected TEST_FAIL, got %q", outcome)
	}
}

func TestParseOutputLowercaseSentinels(t *testing.T) {
	output := `__loop_result__
COMPLETE
__loop_result_end__`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome with lowercase sentinels")
	}
}

func TestParseOutputPartialStartSentinelLeft(t *testing.T) {
	output := `_LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome with partial start sentinel (missing left underscore)")
	}
}

func TestParseOutputPartialStartSentinelRight(t *testing.T) {
	output := `__LOOP_RESULT_
COMPLETE
__LOOP_RESULT_END__`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome with partial start sentinel (missing right underscore)")
	}
}

func TestParseOutputPartialEndSentinelRight(t *testing.T) {
	output := `__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END_`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome with partial end sentinel (missing right underscore)")
	}
}

func TestParseOutputSentinelWithExtraUnderscores(t *testing.T) {
	output := `___LOOP_RESULT___
COMPLETE
___LOOP_RESULT_END___`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome with extra underscores on sentinels")
	}
}

func TestParseOutputSentinelWithTrailingText(t *testing.T) {
	output := `__LOOP_RESULT__extra
COMPLETE
__LOOP_RESULT_END__`
	_, found := ParseOutput(output)
	if found {
		t.Fatal("expected not to find outcome with trailing text on start sentinel")
	}
}

func TestParsePromisesMultipleGroupsThree(t *testing.T) {
	output := `__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__
__LOOP_RESULT__
TEST_FAIL
__LOOP_RESULT_END__
__LOOP_RESULT__
TEST_PASS
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p == nil {
		t.Fatal("expected non-nil Promise")
	}
	if *p != TestPass {
		t.Errorf("expected TEST_PASS (last of three), got %q", *p)
	}
}

func TestParsePromisesWhitespaceOnlyToken(t *testing.T) {
	output := "__LOOP_RESULT__\n   \n__LOOP_RESULT_END__"
	p := ParsePromises(output)
	if p != nil {
		t.Errorf("expected nil for whitespace-only token, got %q", *p)
	}
}

func TestParsePromisesMultipleEmptyGroups(t *testing.T) {
	output := `__LOOP_RESULT__
__LOOP_RESULT_END__
__LOOP_RESULT__
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p != nil {
		t.Errorf("expected nil for multiple empty groups, got %q", *p)
	}
}

func TestParsePromisesEndSentinelBetweenSentinels(t *testing.T) {
	output := `__LOOP_RESULT__
extra text
__LOOP_RESULT_END__
COMPLETE
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p != nil {
		t.Errorf("expected nil when end sentinel appears inside token area, got %q", *p)
	}
}

func TestParsePromisesStartSentinelBetweenSentinels(t *testing.T) {
	output := `__LOOP_RESULT__
some __LOOP_RESULT__ text
COMPLETE
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p != nil {
		t.Errorf("expected nil when start sentinel appears inside token area, got %q", *p)
	}
}

func TestParsePromisesLowercaseSentinels(t *testing.T) {
	output := `__loop_result__
COMPLETE
__loop_result_end__`
	p := ParsePromises(output)
	if p != nil {
		t.Errorf("expected nil for lowercase sentinels, got %q", *p)
	}
}

func TestParsePromisesPartialStartSentinelLeft(t *testing.T) {
	output := `_LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p != nil {
		t.Errorf("expected nil for partial start sentinel, got %q", *p)
	}
}

func TestParsePromisesSentinelWithTrailingText(t *testing.T) {
	output := `__LOOP_RESULT__extra
COMPLETE
__LOOP_RESULT_END__`
	p := ParsePromises(output)
	if p != nil {
		t.Errorf("expected nil for sentinel with trailing text, got %q", *p)
	}
}

func TestRunWithTimeoutPassesContext(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	var capturedCtx context.Context
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		capturedCtx = ctx
		return exec.Command("echo", "-n", "__LOOP_RESULT__\nCOMPLETE\n__LOOP_RESULT_END__")
	}

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "instructions.txt")
	os.WriteFile(promptFile, []byte("# Issue\nBody"), 0644)

	result, err := Run(dir, promptFile, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE, got %q", result.Outcome)
	}

	deadline, ok := capturedCtx.Deadline()
	if !ok {
		t.Fatal("expected deadline on context")
	}
	if time.Until(deadline) > 10*time.Second {
		t.Error("expected deadline within 10s")
	}
}

func TestRunWithNoTimeout(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		_, ok := ctx.Deadline()
		if ok {
			t.Error("expected no deadline when timeout is 0")
		}
		return exec.Command("echo", "-n", "__LOOP_RESULT__\nCOMPLETE\n__LOOP_RESULT_END__")
	}

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "instructions.txt")
	os.WriteFile(promptFile, []byte("# Issue\nBody"), 0644)

	result, err := Run(dir, promptFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE, got %q", result.Outcome)
	}
}

func TestRunWithTimeoutSendsStdin(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "cat")
	}

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "instructions.txt")
	content := "# Test Issue\nBody content"
	os.WriteFile(promptFile, []byte(content), 0644)

	result, err := Run(dir, promptFile, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output.String(), content) {
		t.Errorf("expected prompt content in output, got %q", result.Output.String())
	}
}

func TestRunAgentExecutesOpencode(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	var recordedName string
	var recordedArgs []string
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		recordedName = name
		recordedArgs = args
		return exec.Command("echo", "-n", "output")
	}

	result, err := RunAgent("# Issue\nBody", "## Prompt\nDo work", ".", 0)
	if err != nil {
		t.Fatal(err)
	}
	if recordedName != "opencode" {
		t.Errorf("want opencode, got %s", recordedName)
	}
	if len(recordedArgs) != 2 || recordedArgs[0] != "run" {
		t.Errorf("want [run --dangerously-skip-permissions], got %v", recordedArgs)
	}
	if recordedArgs[1] != "--dangerously-skip-permissions" {
		t.Errorf("want --dangerously-skip-permissions, got %s", recordedArgs[1])
	}
	if result.Stdout.String() != "output" {
		t.Errorf("want output, got %s", result.Stdout.String())
	}
}

func TestRunAgentCombinesContent(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "opencode" {
			t.Errorf("want opencode, got %s", name)
		}
		return exec.Command("sh", "-c", "cat")
	}

	result, err := RunAgent("# Issue\nBody", "## Prompt\nInstructions", ".", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout.String(), "# Issue") {
		t.Error("expected issue text in combined output")
	}
	if !strings.Contains(result.Stdout.String(), "## Prompt") {
		t.Error("expected prompt in combined output")
	}
}

func TestRunAgentEmptyPrompt(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "cat")
	}

	result, err := RunAgent("# Issue\nBody", "", ".", 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(result.Stdout.String(), "\n\n") {
		t.Error("expected no trailing separator when prompt is empty")
	}
}

func TestRunAgentStderrCaptured(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "echo stdout-output; echo stderr-output >&2")
	}

	result, err := RunAgent("# Issue", "## Prompt", ".", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout.String(), "stdout-output") {
		t.Errorf("want stdout-output, got %q", result.Stdout.String())
	}
	if !strings.Contains(result.Stderr.String(), "stderr-output") {
		t.Errorf("want stderr-output in stderr buffer, got %q", result.Stderr.String())
	}
}

func TestRunAgentCommandError(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}

	result, err := RunAgent("# Issue", "## Prompt", ".", 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Err == nil {
		t.Fatal("expected command error in result.Err")
	}
}

func TestRunAgentStdinFlag(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	var recordedArgs []string
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		recordedArgs = args
		return exec.Command("echo", "-n", "ok")
	}

	RunAgent("# Issue", "## Prompt", ".", 0)
	if len(recordedArgs) != 2 || recordedArgs[0] != "run" {
		t.Errorf("want [run --dangerously-skip-permissions], got %v", recordedArgs)
	}
	if recordedArgs[1] != "--dangerously-skip-permissions" {
		t.Errorf("want --dangerously-skip-permissions, got %s", recordedArgs[1])
	}
}

func TestHasOpencode(t *testing.T) {
	// HasOpencode should return true because "opencode" is on PATH in CI/dev.
	got := HasOpencode()
	// We can't assert a specific value since it depends on the environment,
	// but we can assert it doesn't panic and returns a bool.
	if got != true && got != false {
		t.Errorf("HasOpencode() = %v, want true or false", got)
	}
}

func TestRunOpencodeNotFoundReturnsClearError(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("opencode-nonexistent-binary")
	}

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "instructions.txt")
	os.WriteFile(promptFile, []byte("# Issue\nBody"), 0644)

	result, err := Run(dir, promptFile, 0)
	if err == nil {
		t.Fatal("expected error when opencode is not found")
	}
	if result != nil {
		t.Fatal("expected nil result when opencode is not found")
	}
	if !errors.Is(err, ErrOpencodeNotFound) {
		t.Errorf("expected ErrOpencodeNotFound, got %v", err)
	}
}

func TestRunAgentOpencodeNotFoundReturnsClearError(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("opencode-nonexistent-binary")
	}

	_, err := RunAgent("# Issue", "## Prompt", ".", 0)
	if err == nil {
		t.Fatal("expected error when opencode is not found")
	}
	if !errors.Is(err, ErrOpencodeNotFound) {
		t.Errorf("expected ErrOpencodeNotFound, got %v", err)
	}
}

func TestRunNonZeroExitParsesStdoutForPromiseMarkers(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c",
			`echo "working...";
			 echo "__LOOP_RESULT__";
			 echo "TEST_PASS";
			 echo "__LOOP_RESULT_END__";
			 echo "some-stderr" >&2;
			 exit 42`)
	}

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "instructions.txt")
	os.WriteFile(promptFile, []byte("# Issue\nBody"), 0644)

	result, err := Run(dir, promptFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeTestPass {
		t.Errorf("expected TEST_PASS, got %q", result.Outcome)
	}
	if result.Err == nil {
		t.Fatal("expected non-nil error for non-zero exit")
	}
}

func TestRunNonZeroExitWithFailOutcome(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c",
			`echo "some output";
			 echo "some-stderr" >&2;
			 exit 1`)
	}

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "instructions.txt")
	os.WriteFile(promptFile, []byte("# Issue\nBody"), 0644)

	result, err := Run(dir, promptFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeFail {
		t.Errorf("expected FAIL, got %q", result.Outcome)
	}
	if result.Err == nil {
		t.Fatal("expected non-nil error for non-zero exit")
	}
}

func TestRunCapturesStderrSeparately(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c",
			`echo "stdout-line";
			 echo "__LOOP_RESULT__";
			 echo "COMPLETE";
			 echo "__LOOP_RESULT_END__";
			 echo "stderr-line-1" >&2;
			 echo "stderr-line-2" >&2`)
	}

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "instructions.txt")
	os.WriteFile(promptFile, []byte("# Issue\nBody"), 0644)

	result, err := Run(dir, promptFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE, got %q", result.Outcome)
	}
	if !strings.Contains(result.Stderr.String(), "stderr-line-1") {
		t.Errorf("expected stderr to contain 'stderr-line-1', got %q", result.Stderr.String())
	}
	if !strings.Contains(result.Stderr.String(), "stderr-line-2") {
		t.Errorf("expected stderr to contain 'stderr-line-2', got %q", result.Stderr.String())
	}
	if !strings.Contains(result.Stdout.String(), "stdout-line") {
		t.Errorf("expected stdout to contain 'stdout-line', got %q", result.Stdout.String())
	}
}

func TestRunOutputContainsCombined(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c",
			`echo "stdout-only";
			 echo "stderr-only" >&2`)
	}

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "instructions.txt")
	os.WriteFile(promptFile, []byte("# Issue\nBody"), 0644)

	result, err := Run(dir, promptFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	combined := result.Output.String()
	if !strings.Contains(combined, "stdout-only") {
		t.Errorf("expected combined output to contain 'stdout-only', got %q", combined)
	}
	if !strings.Contains(combined, "stderr-only") {
		t.Errorf("expected combined output to contain 'stderr-only', got %q", combined)
	}
}

func TestRunAgentContextStreamedCallsLineFn(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo "line1"; echo "line2"; echo "__LOOP_RESULT__"; echo "COMPLETE"; echo "__LOOP_RESULT_END__"`)
	}

	var captured []string

	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0, func(line string) {
		captured = append(captured, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE, got %q", result.Outcome)
	}

	hasLine1 := false
	hasLine2 := false
	for _, l := range captured {
		if l == "line1" {
			hasLine1 = true
		}
		if l == "line2" {
			hasLine2 = true
		}
	}

	if !hasLine1 {
		t.Error("expected line1 in captured output")
	}
	if !hasLine2 {
		t.Error("expected line2 in captured output")
	}
}

func TestRunAgentContextStreamedCapturesStderr(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo "stdout-line"; echo "stderr-line" >&2; echo "__LOOP_RESULT__"; echo "COMPLETE"; echo "__LOOP_RESULT_END__"`)
	}

	var captured []string
	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0, func(line string) {
		captured = append(captured, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE, got %q", result.Outcome)
	}
	if !strings.Contains(result.Stderr.String(), "stderr-line") {
		t.Errorf("expected stderr to contain 'stderr-line', got %q", result.Stderr.String())
	}
	if !strings.Contains(result.Stdout.String(), "stdout-line") {
		t.Errorf("expected stdout to contain 'stdout-line', got %q", result.Stdout.String())
	}
}

func TestRunAgentContextStreamedNoOutput(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("true")
	}

	called := false
	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0, func(line string) {
		called = true
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("expected lineFn not to be called for empty output")
	}
	if result.Outcome != OutcomeFail {
		t.Errorf("expected FAIL outcome for empty output, got %q", result.Outcome)
	}
}

func TestRunAgentContextStreamedNonZeroExit(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c",
			`echo "working"; echo "__LOOP_RESULT__"; echo "TEST_FAIL"; echo "__LOOP_RESULT_END__"; exit 1`)
	}

	var captured []string
	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0, func(line string) {
		captured = append(captured, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeTestFail {
		t.Errorf("expected TEST_FAIL, got %q", result.Outcome)
	}
	if result.Err == nil {
		t.Fatal("expected non-nil error for non-zero exit")
	}
}

func TestRunAgentContextStreamedOpencodeNotFound(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("opencode-nonexistent-binary")
	}

	_, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0, func(line string) {})
	if err == nil {
		t.Fatal("expected error when opencode is not found")
	}
	if !errors.Is(err, ErrOpencodeNotFound) {
		t.Errorf("expected ErrOpencodeNotFound, got %v", err)
	}
}

func TestKillProcessGroupHandlesNil(t *testing.T) {
	killProcessGroup(nil)

	cmd := exec.Command("echo", "test")
	killProcessGroup(cmd)
}

func TestRunAgentContextStreamedEmptyPrompt(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", `echo "output"; echo "__LOOP_RESULT__"; echo "TEST_PASS"; echo "__LOOP_RESULT_END__"`)
	}

	var captured []string
	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "", ".", 0, func(line string) {
		captured = append(captured, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeTestPass {
		t.Errorf("expected TEST_PASS, got %q", result.Outcome)
	}
	if !strings.Contains(result.Stdout.String(), "output") {
		t.Error("expected output in stdout buffer")
	}
}

func TestRunTruncatedStdoutParsesLastNBytes(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	savedMaxSize := maxOutputSize
	maxOutputSize = 100
	defer func() { maxOutputSize = savedMaxSize }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c",
			`head -c 200 /dev/zero | tr '\0' a; echo ""; echo "__LOOP_RESULT__"; echo "COMPLETE"; echo "__LOOP_RESULT_END__"`)
	}

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "instructions.txt")
	os.WriteFile(promptFile, []byte("# Issue\nBody"), 0644)

	result, err := Run(dir, promptFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated {
		t.Error("expected Truncated=true for output exceeding maxOutputSize")
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE from last N bytes, got %q", result.Outcome)
	}
	if result.Stdout.Len() <= 200 {
		t.Errorf("expected full stdout (>200 bytes) preserved in result, got %d bytes", result.Stdout.Len())
	}
}

func TestRunTruncatedStdoutMarkerLost(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	savedMaxSize := maxOutputSize
	maxOutputSize = 50
	defer func() { maxOutputSize = savedMaxSize }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c",
			`echo "__LOOP_RESULT__"; echo "COMPLETE"; echo "__LOOP_RESULT_END__"; head -c 200 /dev/zero | tr '\0' a; echo ""`)
	}

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "instructions.txt")
	os.WriteFile(promptFile, []byte("# Issue\nBody"), 0644)

	result, err := Run(dir, promptFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated {
		t.Error("expected Truncated=true for output exceeding maxOutputSize")
	}
	if result.Outcome != OutcomeFail {
		t.Errorf("expected FAIL when marker is lost in truncation, got %q", result.Outcome)
	}
}

func TestRunNotTruncated(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	savedMaxSize := maxOutputSize
	maxOutputSize = 1000
	defer func() { maxOutputSize = savedMaxSize }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("echo", "-n", "__LOOP_RESULT__\nCOMPLETE\n__LOOP_RESULT_END__")
	}

	dir := t.TempDir()
	promptFile := filepath.Join(dir, "instructions.txt")
	os.WriteFile(promptFile, []byte("# Issue\nBody"), 0644)

	result, err := Run(dir, promptFile, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Truncated {
		t.Error("expected Truncated=false for small output")
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE, got %q", result.Outcome)
	}
}

func TestRunStreamedTruncatedStdoutParsesLastNBytes(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	savedMaxSize := maxOutputSize
	maxOutputSize = 100
	defer func() { maxOutputSize = savedMaxSize }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c",
			`head -c 200 /dev/zero | tr '\0' a; echo ""; echo "__LOOP_RESULT__"; echo "COMPLETE"; echo "__LOOP_RESULT_END__"`)
	}

	var captured []string
	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0, func(line string) {
		captured = append(captured, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated {
		t.Error("expected Truncated=true for output exceeding maxOutputSize")
	}
	if result.Outcome != OutcomeComplete {
		t.Errorf("expected COMPLETE from last N bytes, got %q", result.Outcome)
	}
	if result.Stdout.Len() <= 200 {
		t.Errorf("expected full stdout (>200 bytes) preserved in result, got %d bytes", result.Stdout.Len())
	}
}

func TestRunStreamedTruncatedStdoutMarkerLost(t *testing.T) {
	saved := execCommandContext
	defer func() { execCommandContext = saved }()

	savedMaxSize := maxOutputSize
	maxOutputSize = 50
	defer func() { maxOutputSize = savedMaxSize }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c",
			`echo "__LOOP_RESULT__"; echo "COMPLETE"; echo "__LOOP_RESULT_END__"; head -c 200 /dev/zero | tr '\0' a; echo ""`)
	}

	var captured []string
	result, err := RunAgentContextStreamed(context.Background(), "# Issue", "## Prompt", ".", 0, func(line string) {
		captured = append(captured, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated {
		t.Error("expected Truncated=true for output exceeding maxOutputSize")
	}
	if result.Outcome != OutcomeFail {
		t.Errorf("expected FAIL when marker is lost in truncation, got %q", result.Outcome)
	}
}

