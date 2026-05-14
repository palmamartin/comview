package diff

import "testing"

func TestRowsWithOptionsAddsInlineSpansForReplacementLines(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-foo := oldValue + 1
+foo := newValue + 1
`)
	if err != nil {
		t.Fatal(err)
	}

	rows := doc.Rows()
	var deleteRow, addRow Row
	for _, row := range rows {
		switch row.Kind {
		case RowDelete:
			deleteRow = row
		case RowAdd:
			addRow = row
		}
	}

	assertSpan(t, deleteRow.InlineSpans, 7, 15)
	assertSpan(t, addRow.InlineSpans, 7, 15)
}

func TestRowsWithOptionsDoesNotHighlightEqualReplacementLines(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-foo := value
+foo := value
`)
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range doc.Rows() {
		if len(row.InlineSpans) != 0 {
			t.Fatalf("row %q has inline spans %+v", row.Text, row.InlineSpans)
		}
	}
}

func TestInlineSpansCoalesceAcrossWhitespace(t *testing.T) {
	oldSpans, newSpans := inlineSpans("old value", "new thing")

	assertSpan(t, oldSpans, 0, len("old value"))
	assertSpan(t, newSpans, 0, len("new thing"))
}

func assertSpan(t *testing.T, spans []InlineSpan, start int, end int) {
	t.Helper()
	if len(spans) != 1 {
		t.Fatalf("spans = %+v, want one span", spans)
	}
	if spans[0].Start != start || spans[0].End != end {
		t.Fatalf("span = %+v, want %d:%d", spans[0], start, end)
	}
}
