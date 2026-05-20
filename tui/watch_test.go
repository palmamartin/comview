package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunWatchCommandDropsANSISequences(t *testing.T) {
	commandPath := filepath.Join(t.TempDir(), "ansi-diff")
	commandSource := "#!/bin/sh\nprintf 'diff --git a/main.go b/main.go\\n\\033[31m-old\\033[0m\\n\\033[32m+new\\033[0m\\n'\n"
	if err := os.WriteFile(commandPath, []byte(commandSource), 0o755); err != nil {
		t.Fatal(err)
	}

	output, err := runWatchCommand(context.Background(), []string{commandPath})
	if err != nil {
		t.Fatalf("runWatchCommand() error = %v", err)
	}

	want := "diff --git a/main.go b/main.go\n-old\n+new\n"
	if output != want {
		t.Fatalf("runWatchCommand() = %q, want %q", output, want)
	}
}
