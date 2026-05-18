package tui

import (
	"path/filepath"
	"testing"
)

func TestExpandHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got, want := expandHomePath("~/comments.json"), filepath.Join(home, "comments.json"); got != want {
		t.Fatalf("expanded path = %q, want %q", got, want)
	}
	if got, want := expandHomePath("~"), home; got != want {
		t.Fatalf("expanded home = %q, want %q", got, want)
	}
	if got, want := expandHomePath("~other/comments.json"), "~other/comments.json"; got != want {
		t.Fatalf("expanded other user path = %q, want %q", got, want)
	}
}

func TestConfigFilePathExpandsXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "~/.config")

	want := filepath.Join(home, ".config", "comview", "config.json")
	if got := configFilePath(); got != want {
		t.Fatalf("config path = %q, want %q", got, want)
	}
}
