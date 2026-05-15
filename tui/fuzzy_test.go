package tui

import "testing"

func TestFuzzyFinderMatchesSubsequence(t *testing.T) {
	finder := newFuzzyFinder("Files", []fuzzyItem{
		{Label: "cmd/comview/main.go", Row: 1},
		{Label: "tui/app.go", Row: 2},
		{Label: "diff/render.go", Row: 3},
	})
	finder.SetQuery("ta")

	matches := finder.Matches()
	if len(matches) != 1 {
		t.Fatalf("matches = %+v, want one match", matches)
	}
	if matches[0].Item.Label != "tui/app.go" {
		t.Fatalf("match = %q, want tui/app.go", matches[0].Item.Label)
	}
}

func TestFuzzyFinderMoveClampsToMatches(t *testing.T) {
	finder := newFuzzyFinder("Files", []fuzzyItem{
		{Label: "a.go"},
		{Label: "b.go"},
	})

	finder.Move(10)
	if finder.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", finder.Cursor)
	}
	finder.Move(-10)
	if finder.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", finder.Cursor)
	}
}
