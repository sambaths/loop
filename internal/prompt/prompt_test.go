package prompt

import (
	"strings"
	"testing"
)

func TestGetPromptNotEmpty(t *testing.T) {
	if GetPrompt() == "" {
		t.Fatal("GetPrompt() must not be empty")
	}
}

func TestGetPromptContainsSections(t *testing.T) {
	sections := []string{
		"ISSUES",
		"SCOPE DISCIPLINE",
		"SENTINEL PROTOCOL",
		"SECTION LIFECYCLE",
		"IMPLEMENTING AGENT",
		"TESTING SUBAGENT",
		"CLOSE-AFTER-TEST",
		"NO MORE TASKS",
		"FINAL RULES",
	}
	for _, s := range sections {
		if !strings.Contains(GetPrompt(), "# "+s) {
			t.Errorf("GetPrompt() missing section %q", s)
		}
	}
}

func TestGetPromptContainsPromiseMarkers(t *testing.T) {
	markers := []string{
		"COMPLETE",
		"TEST_PASS",
		"TEST_FAIL",
		"NO_MORE_TASKS",
		"__LOOP_RESULT__",
		"__LOOP_RESULT_END__",
	}
	for _, m := range markers {
		if !strings.Contains(GetPrompt(), m) {
			t.Errorf("GetPrompt() missing promise marker %q", m)
		}
	}
}

func TestGetPromptContainsSentinelWrapping(t *testing.T) {
	if !strings.Contains(GetPrompt(), "__LOOP_RESULT__\nCOMPLETE\n__LOOP_RESULT_END__") {
		t.Error("GetPrompt() missing sentinel-wrapped promise pattern")
	}
}

func TestGetPromptContainsSectionLifecycle(t *testing.T) {
	if !strings.Contains(GetPrompt(), "SECTION LIFECYCLE") {
		t.Error("GetPrompt() missing SECTION LIFECYCLE section")
	}
	checks := []string{
		"IMPLEMENTING AGENT writes",
		"## Test Results",
		"outputting COMPLETE",
		"TESTING SUBAGENT writes",
		"## UAT Results",
		"outputting TEST_PASS or TEST_FAIL",
		"TEST_FAIL, loop strips",
	}
	for _, c := range checks {
		if !strings.Contains(GetPrompt(), c) {
			t.Errorf("GetPrompt() missing section lifecycle detail %q", c)
		}
	}
}

func TestGetPromptContainsFinalRules(t *testing.T) {
	rules := []string{
		"ONLY WORK ON A SINGLE TASK",
		"NEVER close a GitHub issue",
		"exactly ONE promise marker per iteration",
	}
	for _, r := range rules {
		if !strings.Contains(GetPrompt(), r) {
			t.Errorf("GetPrompt() missing final rule %q", r)
		}
	}
}

func TestGetPromptContainsRoleExecutionModeBranch(t *testing.T) {
	subsections := []string{
		"## Role",
		"## Execution Mode",
		"## Branch",
	}
	for _, s := range subsections {
		if !strings.Contains(GetPrompt(), s) {
			t.Errorf("GetPrompt() missing subsection %q", s)
		}
	}
}

func TestGetPromptExecutionModeCoversHITLAndCombo(t *testing.T) {
	if !strings.Contains(GetPrompt(), "HITL-only") {
		t.Error("GetPrompt() Execution Mode section should mention HITL-only")
	}
	if !strings.Contains(GetPrompt(), "Combo") {
		t.Error("GetPrompt() Execution Mode section should mention Combo")
	}
	if !strings.Contains(GetPrompt(), "output NO_MORE_TASKS") {
		t.Error("GetPrompt() Execution Mode section should instruct outputting NO_MORE_TASKS")
	}
}

func TestGetPromptNoMoreTasksCoversHITLAndCombo(t *testing.T) {
	if !strings.Contains(GetPrompt(), "HITL-only/Combo") {
		t.Error("GetPrompt() NO MORE TASKS section should mention both HITL-only and Combo")
	}
}

