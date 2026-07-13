package gh

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func RunGH(args ...string) (string, error) {
	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command("gh", args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if errors.Is(err, exec.ErrNotFound) {
		return "", fmt.Errorf("gh is not installed or not found in PATH — install it from https://cli.github.com/")
	}
	if err != nil {
		return "", fmt.Errorf("gh %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimSpace(outBuf.String()), nil
}

func HasGH() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func IsAuthenticated() bool {
	_, err := RunGH("auth", "status")
	return err == nil
}

func IssueExists(num int) bool {
	_, err := RunGH("issue", "view", fmt.Sprint(num))
	return err == nil
}

func AddLabel(num int, label string) error {
	_, err := RunGH("issue", "edit", fmt.Sprint(num), "--add-label", label)
	return err
}

func RemoveLabel(num int, label string) error {
	_, err := RunGH("issue", "edit", fmt.Sprint(num), "--remove-label", label)
	return err
}

func IssueState(num int) string {
	state, err := RunGH("issue", "view", fmt.Sprint(num), "--json", "state", "--jq", ".state")
	if err != nil {
		return ""
	}
	return state
}

func CloseIssue(num int, comment string) error {
	args := []string{"issue", "close", fmt.Sprint(num)}
	if comment != "" {
		args = append(args, "--comment", comment)
	}
	_, err := RunGH(args...)
	return err
}

func ReopenIssue(num int) error {
	_, err := RunGH("issue", "reopen", fmt.Sprint(num))
	return err
}

func CommentOnIssue(num int, body string) error {
	_, err := RunGH("issue", "comment", fmt.Sprint(num), "--body", body)
	return err
}
