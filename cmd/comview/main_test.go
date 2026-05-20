package main

import (
	"reflect"
	"testing"
)

func TestWatchCommandDefaultsToGitDiff(t *testing.T) {
	command, err := watchCommand(nil)
	if err != nil {
		t.Fatalf("watchCommand() error = %v", err)
	}
	if want := []string{"git", "diff"}; !reflect.DeepEqual(command, want) {
		t.Fatalf("watchCommand() = %#v, want %#v", command, want)
	}
}

func TestWatchCommandPassesArgsToGitDiff(t *testing.T) {
	command, err := watchCommand([]string{"--staged", "HEAD~1"})
	if err != nil {
		t.Fatalf("watchCommand() error = %v", err)
	}
	if want := []string{"git", "diff", "--staged", "HEAD~1"}; !reflect.DeepEqual(command, want) {
		t.Fatalf("watchCommand() = %#v, want %#v", command, want)
	}
}

func TestWatchCommandAcceptsCustomCommandAfterSeparator(t *testing.T) {
	command, err := watchCommand([]string{"--", "gh", "pr", "diff", "123"})
	if err != nil {
		t.Fatalf("watchCommand() error = %v", err)
	}
	if want := []string{"gh", "pr", "diff", "123"}; !reflect.DeepEqual(command, want) {
		t.Fatalf("watchCommand() = %#v, want %#v", command, want)
	}
}

func TestWatchCommandRequiresCommandAfterSeparator(t *testing.T) {
	if _, err := watchCommand([]string{"--"}); err == nil {
		t.Fatal("watchCommand() error = nil, want error")
	}
}
