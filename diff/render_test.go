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

	if deleteRow.Text != "     1        │ -old" {
		t.Fatalf("delete row = %q", deleteRow.Text)
	}
	if addRow.Text != "            1 │ +new" {
		t.Fatalf("add row = %q", addRow.Text)
	}
	if contextRow.Text != "     2      2 │  same" {
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

	if deleteRow.Text != "1   │ -old" {
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

	if deleteRow.Text != "  1     │ -old" {
		t.Fatalf("delete row = %q", deleteRow.Text)
	}
}

func TestDefaultRenderOptionsShowLineNumbers(t *testing.T) {
	options := DefaultRenderOptions()
	if !options.ShowLineNumbers {
		t.Fatal("DefaultRenderOptions().ShowLineNumbers = false, want true")
	}
}
