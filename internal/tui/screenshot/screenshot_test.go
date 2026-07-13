package screenshot

import (
	"os"
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b[1mbold\x1b[22m", "bold"},
		{"a\x1b[31mb\x1b[0mc", "abc"},
		{"", ""},
		{"\x1b[38;2;255;0;0mcolored\x1b[0m", "colored"},
	}
	for _, tt := range tests {
		got := StripANSI(tt.input)
		if got != tt.expected {
			t.Errorf("StripANSI(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSaveCreatesFile(t *testing.T) {
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	dir := t.TempDir()
	os.Chdir(dir)

	name, err := Save("hello world", "test")
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if !strings.HasPrefix(name, "loop-screenshot-") {
		t.Errorf("expected filename to start with loop-screenshot-, got %q", name)
	}
	if !strings.HasSuffix(name, ".txt") {
		t.Errorf("expected filename to end with .txt, got %q", name)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected content 'hello world', got %q", string(data))
	}
}

func TestSaveStripsANSI(t *testing.T) {
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	dir := t.TempDir()
	os.Chdir(dir)

	name, err := Save("\x1b[32mgreen\x1b[0m text", "test")
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "green text" {
		t.Errorf("expected content 'green text', got %q", string(data))
	}
}
