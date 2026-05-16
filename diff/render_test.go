package diff

import (
	"strings"
	"testing"

	"github.com/rockorager/comview/review"
)

func TestRowsWithOptionsCanHideMetadataAndContext(t *testing.T) {
	doc, err := Parse(`commit abc123

diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
 package main
-old
+new
 unchanged
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.RowsWithOptions(RenderOptions{
		ShowFileHeaders: true,
		ShowHunkHeaders: true,
	})

	for _, row := range rows {
		switch row.Kind {
		case RowPreamble, RowMeta, RowContext:
			t.Fatalf("unexpected row kind %v with text %q", row.Kind, row.Text)
		}
	}
}

func TestRowsWithOptionsAddsReviewAnchors(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -10,3 +20,3 @@
 context
-old
+new
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.RowsWithOptions(RenderOptions{
		ShowHunkHeaders: true,
		ShowContext:     true,
		ShowLineNumbers: true,
	})

	var contextRow, deleteRow, addRow Row
	for _, row := range rows {
		switch row.Kind {
		case RowContext:
			contextRow = row
		case RowDelete:
			deleteRow = row
		case RowAdd:
			addRow = row
		}
	}

	if got, want := contextRow.Review, (review.Anchor{Path: "main.go", Line: 20, Side: review.SideRight}); got != want {
		t.Fatalf("context review anchor = %+v, want %+v", got, want)
	}
	if got, want := deleteRow.Review, (review.Anchor{Path: "main.go", Line: 11, Side: review.SideLeft}); got != want {
		t.Fatalf("delete review anchor = %+v, want %+v", got, want)
	}
	if got, want := addRow.Review, (review.Anchor{Path: "main.go", Line: 21, Side: review.SideRight}); got != want {
		t.Fatalf("add review anchor = %+v, want %+v", got, want)
	}
}

func TestRowsWithOptionsAddsCommitIDToReviewAnchors(t *testing.T) {
	doc, err := Parse(`commit abc123

diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-old
+new
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()

	for _, row := range rows {
		if row.Kind == RowAdd {
			if got, want := row.Review.CommitID, "abc123"; got != want {
				t.Fatalf("commit id = %q, want %q", got, want)
			}
			return
		}
	}
	t.Fatal("add row not found")
}

func TestRowsWithOptionsHighlightsGitShowPreamble(t *testing.T) {
	doc, err := Parse(`commit abc123
Author: Example <example@example.com>
Date:   Thu May 14 12:00:00 2026 -0500

    Add commit highlighting

    Reviewed-by: Someone <someone@example.com>

diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-old
+new
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()

	tests := []struct {
		index  int
		kind   RowKind
		prefix string
		code   string
	}{
		{0, RowCommitHeader, "commit ", "abc123"},
		{1, RowCommitMeta, "Author: ", "Example <example@example.com>"},
		{2, RowCommitMeta, "Date:   ", "Thu May 14 12:00:00 2026 -0500"},
		{3, RowBlank, "", ""},
		{4, RowCommitMessage, "", ""},
		{5, RowBlank, "", ""},
		{6, RowCommitTrailer, "    Reviewed-by: ", "Someone <someone@example.com>"},
	}
	for _, tt := range tests {
		row := rows[tt.index]
		if row.Kind != tt.kind || row.Prefix != tt.prefix || row.Code != tt.code {
			t.Fatalf("row %d = kind:%v prefix:%q code:%q text:%q", tt.index, row.Kind, row.Prefix, row.Code, row.Text)
		}
	}
}

func TestRowsWithOptionsRendersDiffStatRows(t *testing.T) {
	doc, err := Parse(`commit abc123
Author: Example <example@example.com>

 README.md        |  1 +
 tui/app.go       | 12 ++++++------
 2 files changed, 7 insertions(+), 6 deletions(-)
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()

	var statRows []Row
	var summary Row
	for _, row := range rows {
		switch row.Kind {
		case RowDiffStat:
			statRows = append(statRows, row)
		case RowDiffStatSummary:
			summary = row
		}
	}
	if len(statRows) != 2 {
		t.Fatalf("stat rows = %+v, want 2", statRows)
	}
	if statRows[0].Stat.Path != "README.md" || statRows[0].Stat.Adds != 1 || statRows[0].Stat.Deletes != 0 {
		t.Fatalf("first stat = %+v", statRows[0].Stat)
	}
	if statRows[1].Stat.Path != "tui/app.go" || statRows[1].Stat.Adds != 6 || statRows[1].Stat.Deletes != 6 {
		t.Fatalf("second stat = %+v", statRows[1].Stat)
	}
	if summary.Kind != RowDiffStatSummary || summary.Stat.Files != 2 || summary.Stat.Adds != 7 || summary.Stat.Deletes != 6 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestRowsWithOptionsInterleavesMultipleCommitPreambles(t *testing.T) {
	doc, err := Parse(`commit abc123
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
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()

	var secondCommit int
	var firstFile int
	var secondFile int
	var blankBeforeSecondCommit bool
	for index, row := range rows {
		switch {
		case row.Kind == RowCommitHeader && row.Code == "def456":
			secondCommit = index
			blankBeforeSecondCommit = index > 0 && rows[index-1].Kind == RowBlank
		case row.Kind == RowFile && row.Text == "a.txt":
			firstFile = index
		case row.Kind == RowFile && row.Text == "b.txt":
			secondFile = index
		}
	}
	if firstFile == 0 || secondCommit == 0 || secondFile == 0 {
		t.Fatalf("missing rows: firstFile=%d secondCommit=%d secondFile=%d rows=%+v", firstFile, secondCommit, secondFile, rows)
	}
	if firstFile >= secondCommit || secondCommit >= secondFile {
		t.Fatalf("row order firstFile=%d secondCommit=%d secondFile=%d", firstFile, secondCommit, secondFile)
	}
	if !blankBeforeSecondCommit {
		t.Fatalf("row before second commit = %+v, want blank", rows[secondCommit-1])
	}
}

func TestRowsWithOptionsUsesPerFileCommitMetadataForMultipleCommits(t *testing.T) {
	doc, err := Parse(`commit abc123
Author: Example <example@example.com>

diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-old
+new
commit def456
Author: Other <other@example.com>

diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-older
+newer
`)
	if err != nil {
		t.Fatal(err)
	}

	var commits []string
	for _, row := range doc.Rows() {
		if row.Kind == RowAdd {
			commits = append(commits, row.Review.CommitID)
		}
	}
	if len(commits) != 2 {
		t.Fatalf("add rows = %d, want 2", len(commits))
	}
	if commits[0] != "abc123" || commits[1] != "def456" {
		t.Fatalf("commit ids = %#v, want abc123/def456", commits)
	}
}

func TestRowsWithOptionsRendersNoNewlineMarkerOutsideCode(t *testing.T) {
	doc, err := Parse(`diff --git a/app.ts b/app.ts
--- a/app.ts
+++ b/app.ts
@@ -1 +1 @@
-const value = "old";
\ No newline at end of file
+const value = "new";
\ No newline at end of file
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()
	var markers int
	for _, row := range rows {
		if row.Kind != RowNoNewline {
			continue
		}
		markers++
		if row.Text != `\ No newline at end of file` {
			t.Fatalf("no-newline text = %q", row.Text)
		}
		if row.Gutter != "" || row.Marker != "" || row.Code != "" {
			t.Fatalf("no-newline row has gutter/marker/code: %+v", row)
		}
		if row.Review != (review.Anchor{}) {
			t.Fatalf("no-newline row review anchor = %+v", row.Review)
		}
	}
	if markers != 2 {
		t.Fatalf("no-newline markers = %d, want 2", markers)
	}
}

func TestRowsWithOptionsRendersEmptyDiffLineAsCodeSpace(t *testing.T) {
	input := strings.Join([]string{
		"diff --git a/app.ts b/app.ts",
		"--- a/app.ts",
		"+++ b/app.ts",
		"@@ -1,2 +1,2 @@",
		" const value = 1;",
		" ",
		"",
	}, "\n")
	doc, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range doc.Rows() {
		if row.Kind != RowContext || row.Review.Line != 2 {
			continue
		}
		if row.Code != " " {
			t.Fatalf("empty context code = %q, want single space", row.Code)
		}
		if row.Text != row.Gutter+" " {
			t.Fatalf("empty context text = %q, want gutter plus space", row.Text)
		}
		return
	}
	t.Fatal("empty context row not found")
}

func TestRowsWithOptionsRendersCRLFEmptyDiffLineAsCodeSpaceBetweenCommits(t *testing.T) {
	input := strings.Join([]string{
		"commit abc123",
		"Author: Example <example@example.com>",
		"",
		"diff --git a/app.ts b/app.ts",
		"--- a/app.ts",
		"+++ b/app.ts",
		"@@ -1,2 +1,2 @@",
		" const value = 1;",
		" ",
		"commit def456",
		"Author: Other <other@example.com>",
		"",
		"diff --git a/next.ts b/next.ts",
		"--- a/next.ts",
		"+++ b/next.ts",
		"@@ -1 +1 @@",
		"-old",
		"+new",
		"",
	}, "\r\n")
	doc, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}

	var emptyContext Row
	var found bool
	for _, row := range doc.Rows() {
		if row.Kind == RowContext && row.Review.Path == "app.ts" && row.Review.Line == 2 {
			emptyContext = row
			found = true
			break
		}
	}
	if !found {
		t.Fatal("empty context row not found")
	}
	if emptyContext.Code != " " {
		t.Fatalf("empty context code = %q, want single space", emptyContext.Code)
	}
	if strings.Contains(emptyContext.Text, "\r") {
		t.Fatalf("empty context text contains carriage return: %q", emptyContext.Text)
	}
}

func TestRowsUseCleanFileHeaderByDefault(t *testing.T) {
	doc, err := Parse(`diff --git a/tui/app.go b/tui/app.go
index b458ab8f49ee..9b18adf56d83 100644
--- a/tui/app.go
+++ b/tui/app.go
@@ -1 +1 @@
-old
+new
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()
	if len(rows) == 0 || rows[0].Kind != RowFile {
		t.Fatalf("first row = %+v, want file row", rows)
	}
	if rows[0].Text != "tui/app.go" {
		t.Fatalf("file row = %q, want tui/app.go", rows[0].Text)
	}
	for _, row := range rows {
		if row.Kind == RowMeta {
			t.Fatalf("unexpected metadata row %q", row.Text)
		}
	}
}

func TestRowsWithOptionsCanShowFileMetadata(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-old
+new
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.RowsWithOptions(RenderOptions{
		ShowFileHeaders:  true,
		ShowFileMetadata: true,
		ShowHunkHeaders:  true,
	})

	var metadata []string
	for _, row := range rows {
		if row.Kind == RowMeta {
			metadata = append(metadata, row.Text)
		}
	}
	if len(metadata) != 3 {
		t.Fatalf("metadata rows = %q, want index/---/+++", metadata)
	}
	if metadata[0] != "index 1111111..2222222 100644" {
		t.Fatalf("first metadata row = %q", metadata[0])
	}
}

func TestRowsUseCleanRenameFileHeader(t *testing.T) {
	doc, err := Parse(`diff --git a/old.go b/new.go
--- a/old.go
+++ b/new.go
@@ -1 +1 @@
-old
+new
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()
	if len(rows) == 0 || rows[0].Text != "old.go -> new.go" {
		t.Fatalf("file row = %+v, want old.go -> new.go", rows)
	}
}

func TestRowsSeparateFileHeaders(t *testing.T) {
	doc, err := Parse(`diff --git a/one.go b/one.go
--- a/one.go
+++ b/one.go
@@ -1 +1 @@
-old
+new
diff --git a/two.go b/two.go
--- a/two.go
+++ b/two.go
@@ -1 +1 @@
-old
+new
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()
	for i, row := range rows {
		if row.Kind != RowFile || row.Text != "two.go" {
			continue
		}
		if i == 0 || rows[i-1].Kind != RowBlank || rows[i-1].Text != "" {
			t.Fatalf("row before second file = %+v, want blank row", rows[i-1])
		}
		return
	}
	t.Fatal("missing second file row")
}

func TestRowsSeparateHunks(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-old
+new
@@ -10 +10 @@
-older
+newer
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()
	hunks := 0
	for i, row := range rows {
		if row.Kind != RowHunk {
			continue
		}
		hunks++
		if hunks == 2 {
			if i == 0 || rows[i-1].Kind != RowBlank {
				t.Fatalf("row before second hunk = %+v, want blank", rows[i-1])
			}
			return
		}
	}
	t.Fatalf("hunks = %d, want 2", hunks)
}

func TestRowsWithOptionsCanShowLineNumbers(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
-old
+new
 same
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.RowsWithOptions(RenderOptions{
		ShowHunkHeaders: true,
		ShowContext:     true,
		ShowLineNumbers: true,
		LineNumberWidth: 6,
	})

	var deleteRow, addRow, contextRow Row
	for _, row := range rows {
		switch row.Kind {
		case RowDelete:
			deleteRow = row
		case RowAdd:
			addRow = row
		case RowContext:
			contextRow = row
		}
	}

	if deleteRow.Text != "     1        - old" {
		t.Fatalf("delete row = %q", deleteRow.Text)
	}
	if addRow.Text != "            1 + new" {
		t.Fatalf("add row = %q", addRow.Text)
	}
	if contextRow.Text != "     2      2   same" {
		t.Fatalf("context row = %q", contextRow.Text)
	}
}

func TestRowsWithOptionsUseCompactLineNumberWidth(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
-old
+new
 same
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()
	var deleteRow Row
	for _, row := range rows {
		if row.Kind == RowDelete {
			deleteRow = row
			break
		}
	}

	if deleteRow.Text != "1   - old" {
		t.Fatalf("delete row = %q", deleteRow.Text)
	}
}

func TestRowsWithOptionsAllowLineNumberWidthOverride(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-old
+new
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.RowsWithOptions(RenderOptions{
		ShowLineNumbers: true,
		LineNumberWidth: 3,
	})
	var deleteRow Row
	for _, row := range rows {
		if row.Kind == RowDelete {
			deleteRow = row
			break
		}
	}

	if deleteRow.Text != "  1     - old" {
		t.Fatalf("delete row = %q", deleteRow.Text)
	}
}

func TestDefaultRenderOptionsShowLineNumbers(t *testing.T) {
	options := DefaultRenderOptions()
	if !options.ShowLineNumbers {
		t.Fatal("DefaultRenderOptions().ShowLineNumbers = false, want true")
	}
	if options.ShowFileMetadata {
		t.Fatal("DefaultRenderOptions().ShowFileMetadata = true, want false")
	}
}

func TestRowsUseSyntaxFileName(t *testing.T) {
	doc, err := Parse(`diff --git a/old.go b/new.go
--- a/old.go
+++ b/new.go
@@ -1 +1 @@
-old
+new
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()
	for _, row := range rows {
		if row.Kind == RowAdd {
			if row.FileName != "new.go" {
				t.Fatalf("add row file name = %q, want new.go", row.FileName)
			}
			return
		}
	}
	t.Fatal("missing add row")
}

func TestRowsSplitHunkHeaderContext(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -106,7 +111,57 @@ func (d *diffViewer) Paint(win vaxis.Window) {
 unchanged
`)
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range doc.Rows() {
		if row.Kind != RowHunk {
			continue
		}
		if row.Prefix != "@@ -106,7 +111,57 @@" {
			t.Fatalf("prefix = %q", row.Prefix)
		}
		if row.Code != " func (d *diffViewer) Paint(win vaxis.Window) {" {
			t.Fatalf("code = %q", row.Code)
		}
		return
	}
	t.Fatal("missing hunk row")
}
