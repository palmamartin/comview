package diff

import (
	"strings"
	"testing"
)

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
	if file.OldObjectID != "1111111" || file.NewObjectID != "2222222" {
		t.Fatalf("object ids = %q, %q", file.OldObjectID, file.NewObjectID)
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

func TestParseGitDiffIndexObjectIDs(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
index 1234567..89abcde 100644
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-old
+new
`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := doc.Files[0].OldObjectID, "1234567"; got != want {
		t.Fatalf("old object id = %q, want %q", got, want)
	}
	if got, want := doc.Files[0].NewObjectID, "89abcde"; got != want {
		t.Fatalf("new object id = %q, want %q", got, want)
	}
}

func TestViewedObjectIDUsesNewObjectIDForModifiedFile(t *testing.T) {
	file := File{OldObjectID: "1234567", NewObjectID: "89abcde"}
	if got, want := viewedObjectID(file), "89abcde"; got != want {
		t.Fatalf("viewed object id = %q, want %q", got, want)
	}
}

func TestViewedObjectIDUsesOldObjectIDForDeletedFile(t *testing.T) {
	file := File{OldObjectID: "1234567", NewObjectID: "0000000"}
	if got, want := viewedObjectID(file), "1234567"; got != want {
		t.Fatalf("viewed object id = %q, want %q", got, want)
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

func TestParseMultipleGitShowCommits(t *testing.T) {
	input := `commit abc123
Author: Example <example@example.com>

diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-a
+b
commit def456
Author: Other <other@example.com>

diff --git a/b.txt b/b.txt
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-c
+d
`

	doc, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(doc.Files), 2; got != want {
		t.Fatalf("files = %d, want %d", got, want)
	}
	if got, want := len(doc.Preamble), 3; got != want {
		t.Fatalf("preamble lines = %d, want %d: %#v", got, want, doc.Preamble)
	}
	if got, want := len(doc.Files[1].Preamble), 3; got != want {
		t.Fatalf("second file preamble lines = %d, want %d: %#v", got, want, doc.Files[1].Preamble)
	}
	if got, want := doc.Files[1].Preamble[0], "commit def456"; got != want {
		t.Fatalf("second commit preamble = %q, want %q", got, want)
	}
	if got, want := doc.Files[0].Metadata.CommitID, "abc123"; got != want {
		t.Fatalf("first file commit id = %q, want %q", got, want)
	}
	if got, want := doc.Files[1].Metadata.CommitID, "def456"; got != want {
		t.Fatalf("second file commit id = %q, want %q", got, want)
	}
	if got, want := len(doc.Files[0].Hunks[0].Lines), 2; got != want {
		t.Fatalf("first hunk lines = %d, want %d", got, want)
	}
}

func TestParseDoesNotTreatBlankCommitSeparatorAsHunkLine(t *testing.T) {
	input := strings.Join([]string{
		"commit abc123",
		"Author: Example <example@example.com>",
		"",
		"diff --git a/a.ts b/a.ts",
		"--- a/a.ts",
		"+++ b/a.ts",
		"@@ -1 +1 @@",
		" ",
		"",
		"commit def456",
		"Author: Other <other@example.com>",
		"",
		"diff --git a/b.ts b/b.ts",
		"--- a/b.ts",
		"+++ b/b.ts",
		"@@ -1 +1 @@",
		"-old",
		"+new",
		"",
	}, "\n")

	doc, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(doc.Files), 2; got != want {
		t.Fatalf("files = %d, want %d", got, want)
	}
	if got, want := len(doc.Files[0].Hunks[0].Lines), 1; got != want {
		t.Fatalf("first hunk lines = %d, want %d: %+v", got, want, doc.Files[0].Hunks[0].Lines)
	}
	line := doc.Files[0].Hunks[0].Lines[0]
	if line.Kind != Context || line.OldLine != 1 || line.NewLine != 1 || line.Text != " " {
		t.Fatalf("first hunk line = %+v, want one blank context line", line)
	}
	if got, want := doc.Files[1].Preamble[0], "commit def456"; got != want {
		t.Fatalf("second preamble starts with %q, want %q", got, want)
	}
}
