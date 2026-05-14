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

func TestRowsWithOptionsDoesNotPairUnrelatedReplacementLines(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-func oldThing() {
+type User struct {
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

func TestRowsWithOptionsDoesNotPairLinesSharingOnlyPunctuation(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-foo.Bar()
+baz.Qux()
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

func TestRowsWithOptionsPairsShiftedSimilarLines(t *testing.T) {
	doc, err := Parse(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,3 @@
-foo := oldValue + 1
-keep()
+inserted()
+foo := newValue + 1
+keep()
`)
	if err != nil {
		t.Fatal(err)
	}

	var deleteRows []Row
	var addRows []Row
	for _, row := range doc.Rows() {
		switch row.Kind {
		case RowDelete:
			deleteRows = append(deleteRows, row)
		case RowAdd:
			addRows = append(addRows, row)
		}
	}

	if len(deleteRows) != 2 || len(addRows) != 3 {
		t.Fatalf("got %d delete rows and %d add rows", len(deleteRows), len(addRows))
	}
	assertSpan(t, deleteRows[0].InlineSpans, 7, 15)
	if len(addRows[0].InlineSpans) != 0 {
		t.Fatalf("inserted row has inline spans %+v", addRows[0].InlineSpans)
	}
	assertSpan(t, addRows[1].InlineSpans, 7, 15)
	if len(deleteRows[1].InlineSpans) != 0 || len(addRows[2].InlineSpans) != 0 {
		t.Fatalf("equal shifted rows have inline spans delete=%+v add=%+v", deleteRows[1].InlineSpans, addRows[2].InlineSpans)
	}
}

func TestRowsWithOptionsHighlightsReplacedAppendArgumentAsOneSpan(t *testing.T) {
	doc, err := Parse(`diff --git a/render.go b/render.go
--- a/render.go
+++ b/render.go
@@ -65 +65 @@
-rows = append(rows, Row{Kind: RowHunk, Text: hunk.Header, FileName: syntaxName})
+rows = append(rows, renderHunkHeaderRow(syntaxName, hunk))
`)
	if err != nil {
		t.Fatal(err)
	}

	var deleteRow, addRow Row
	for _, row := range doc.Rows() {
		switch row.Kind {
		case RowDelete:
			deleteRow = row
		case RowAdd:
			addRow = row
		}
	}

	const unchangedPrefix = "rows = append(rows, "
	assertSpan(t, deleteRow.InlineSpans, len(unchangedPrefix), len(deleteRow.Code)-1)
	assertSpan(t, addRow.InlineSpans, len(unchangedPrefix), len(addRow.Code)-1)
}

func TestRowsWithOptionsPairsOneAddedLineInLargerDeleteBlock(t *testing.T) {
	doc, err := Parse(`diff --git a/app.go b/app.go
--- a/app.go
+++ b/app.go
@@ -467,6 +463,1 @@
-        for row := bar.Row; row < bar.Row+bar.Length; row++ {
-                style := trackStyle
-                grapheme := "│"
-                if row >= bar.Thumb && row < bar.Thumb+bar.Size {
-                        style = thumbStyle
-                        grapheme = verticalScrollbarThumb
+        for row := bar.Thumb; row < bar.Thumb+bar.Size; row++ {
`)
	if err != nil {
		t.Fatal(err)
	}

	var deleteRows []Row
	var addRows []Row
	for _, row := range doc.Rows() {
		switch row.Kind {
		case RowDelete:
			deleteRows = append(deleteRows, row)
		case RowAdd:
			addRows = append(addRows, row)
		}
	}

	if len(deleteRows) != 6 || len(addRows) != 1 {
		t.Fatalf("got %d delete rows and %d add rows", len(deleteRows), len(addRows))
	}
	assertSpanTexts(t, deleteRows[0], "Row", "Row", "Length")
	assertSpanTexts(t, addRows[0], "Thumb", "Thumb", "Size")
	for _, row := range deleteRows[1:] {
		if len(row.InlineSpans) != 0 {
			t.Fatalf("unpaired row %q has inline spans %+v", row.Text, row.InlineSpans)
		}
	}
}

func TestRowsWithOptionsPrefersMatchingLeadingToken(t *testing.T) {
	doc, err := Parse(`diff --git a/inline.go b/inline.go
--- a/inline.go
+++ b/inline.go
@@ -42,1 +42,4 @@
-const minInlineLineSimilarity = 0.45
+const (
+        minInlineLineSimilarity     = 0.45
+        leadingTokenMismatchPenalty = 0.75
+)
`)
	if err != nil {
		t.Fatal(err)
	}

	var deleteRow Row
	var addRows []Row
	for _, row := range doc.Rows() {
		switch row.Kind {
		case RowDelete:
			deleteRow = row
		case RowAdd:
			addRows = append(addRows, row)
		}
	}

	if deleteRow.Code == "" || len(addRows) != 4 {
		t.Fatalf("delete row = %+v, add rows = %+v", deleteRow, addRows)
	}
	if len(deleteRow.InlineSpans) == 0 {
		t.Fatalf("delete row %q has no inline spans", deleteRow.Text)
	}
	for _, span := range deleteRow.InlineSpans {
		if span.Start < len("const") {
			t.Fatalf("delete span %+v highlights leading const in %q", span, deleteRow.Code)
		}
	}
	if len(addRows[0].InlineSpans) == 0 {
		t.Fatalf("leading const row %q has no inline spans", addRows[0].Text)
	}
	for _, row := range addRows[1:] {
		if len(row.InlineSpans) != 0 {
			t.Fatalf("unpaired row %q has inline spans %+v", row.Text, row.InlineSpans)
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

func assertSpanTexts(t *testing.T, row Row, texts ...string) {
	t.Helper()
	if len(row.InlineSpans) != len(texts) {
		t.Fatalf("row %q spans = %+v, want texts %q", row.Text, row.InlineSpans, texts)
	}
	for i, span := range row.InlineSpans {
		if span.Start < 0 || span.End > len(row.Code) || span.Start >= span.End {
			t.Fatalf("row %q has invalid span %+v", row.Text, span)
		}
		if got := row.Code[span.Start:span.End]; got != texts[i] {
			t.Fatalf("span %d text = %q, want %q in row %q", i, got, texts[i], row.Text)
		}
	}
}
