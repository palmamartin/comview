package diff

import "testing"

func TestParseGitDiff(t *testing.T) {
	input := `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
-old
+new
+added
`

	doc, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(doc.Files))
	}
	file := doc.Files[0]
	if file.OldName != "a/main.go" || file.NewName != "b/main.go" {
		t.Fatalf("file names = %q, %q", file.OldName, file.NewName)
	}
	if len(file.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(file.Hunks))
	}
	if got := len(file.Hunks[0].Lines); got != 4 {
		t.Fatalf("hunk lines = %d, want 4", got)
	}
	if file.Hunks[0].Lines[1].Kind != Delete {
		t.Fatalf("second line kind = %v, want Delete", file.Hunks[0].Lines[1].Kind)
	}
	if file.Hunks[0].Lines[2].Kind != Add {
		t.Fatalf("third line kind = %v, want Add", file.Hunks[0].Lines[2].Kind)
	}
}

func TestParseGitShowPreamble(t *testing.T) {
	input := `commit abc123
Author: Example <example@example.com>

diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-a
+b
`

	doc, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Preamble) != 3 {
		t.Fatalf("preamble lines = %d, want 3", len(doc.Preamble))
	}
	if got, want := doc.Metadata.SourceKind, "show"; got != want {
		t.Fatalf("source kind = %q, want %q", got, want)
	}
	if got, want := doc.Metadata.CommitID, "abc123"; got != want {
		t.Fatalf("commit id = %q, want %q", got, want)
	}
	if len(doc.Rows()) == 0 {
		t.Fatal("Rows returned no rows")
	}
}
