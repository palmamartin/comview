package diff

import "testing"

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
