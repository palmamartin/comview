package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sr.ht/~rockorager/vaxis"

	"github.com/rockorager/comview/diff"
	"github.com/rockorager/comview/review"
)

func TestDiffViewerUsesQueriedDiffColors(t *testing.T) {
	viewer := &diffViewer{}
	colors := TerminalColors{
		Red:   vaxis.RGBColor(1, 2, 3),
		Green: vaxis.RGBColor(4, 5, 6),
	}
	viewer.SetTerminalColors(colors)

	if got := viewer.styleFor(diff.RowDelete).Foreground; got != colors.Red {
		t.Fatalf("delete foreground = %v, want %v", got, colors.Red)
	}
	if got := viewer.styleFor(diff.RowAdd).Foreground; got != colors.Green {
		t.Fatalf("add foreground = %v, want %v", got, colors.Green)
	}
}

func TestRowsForInputReturnsNoRowsForEmptyInput(t *testing.T) {
	rows, err := rowsForInput("")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}

func TestRowsForInputReturnsRowsForDiff(t *testing.T) {
	rows, err := rowsForInput(`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-old
+new
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("rows = 0, want diff rows")
	}
}

func TestDiffViewerFallsBackToRGBDiffColors(t *testing.T) {
	viewer := &diffViewer{}
	viewer.ensureColorScheme()

	if got, want := viewer.styleFor(diff.RowDelete).Foreground, DefaultColorScheme().Delete; got != want {
		t.Fatalf("delete foreground = %v, want %v", got, want)
	}
	if got, want := viewer.styleFor(diff.RowAdd).Foreground, DefaultColorScheme().Add; got != want {
		t.Fatalf("add foreground = %v, want %v", got, want)
	}
}

func TestDefaultColorSchemeUsesOnlyRGBColors(t *testing.T) {
	scheme := DefaultColorScheme()
	colors := []vaxis.Color{
		scheme.Base.Foreground,
		scheme.Base.Background,
		scheme.Base.Red,
		scheme.Base.Green,
		scheme.Base.Yellow,
		scheme.Base.Blue,
		scheme.Base.Magenta,
		scheme.Base.Cyan,
		scheme.Foreground,
		scheme.Background,
		scheme.Code,
		scheme.Dim,
		scheme.Header,
		scheme.Muted,
		scheme.Hunk,
		scheme.Gutter,
		scheme.Blue,
		scheme.Yellow,
		scheme.Add,
		scheme.AddLine,
		scheme.AddInline,
		scheme.Delete,
		scheme.DeleteLine,
		scheme.DeleteInline,
		scheme.Selection,
		scheme.Yank,
	}

	for _, color := range colors {
		if params := color.Params(); len(params) != 3 {
			t.Fatalf("color %v has params %v, want RGB params", color, params)
		}
	}
}

func TestColorSchemeDimIsBlendedRGB(t *testing.T) {
	scheme := DefaultColorScheme()
	want := blendRGB(scheme.Foreground, scheme.Background, dimBlend)

	if scheme.Dim != want {
		t.Fatalf("dim = %v, want %v", scheme.Dim, want)
	}
}

func TestChangedLineBackgroundsAreBlendedAndContrasting(t *testing.T) {
	scheme := DefaultColorScheme()

	if scheme.AddLine == scheme.Background {
		t.Fatal("add line background equals base background")
	}
	if scheme.DeleteLine == scheme.Background {
		t.Fatal("delete line background equals base background")
	}
	if got := contrastRatio(scheme.Background, scheme.AddLine); got < minChangedLineContrast {
		t.Fatalf("add line contrast = %f, want at least %f", got, minChangedLineContrast)
	}
	if got := contrastRatio(scheme.Background, scheme.DeleteLine); got < minChangedLineContrast {
		t.Fatalf("delete line contrast = %f, want at least %f", got, minChangedLineContrast)
	}
}

func TestDiffViewerUsesChangedLineBackgrounds(t *testing.T) {
	viewer := &diffViewer{}
	viewer.ensureColorScheme()

	if got, want := viewer.styleFor(diff.RowAdd).Background, viewer.scheme.AddLine; got != want {
		t.Fatalf("add background = %v, want %v", got, want)
	}
	if got, want := viewer.styleFor(diff.RowDelete).Background, viewer.scheme.DeleteLine; got != want {
		t.Fatalf("delete background = %v, want %v", got, want)
	}
}

func TestDiffViewerUsesInlineChangeBackgrounds(t *testing.T) {
	viewer := &diffViewer{}
	viewer.ensureColorScheme()

	if got, want := viewer.inlineBackground(diff.RowAdd), viewer.scheme.AddInline; got != want {
		t.Fatalf("add inline background = %v, want %v", got, want)
	}
	if got, want := viewer.inlineBackground(diff.RowDelete), viewer.scheme.DeleteInline; got != want {
		t.Fatalf("delete inline background = %v, want %v", got, want)
	}
}

func TestDiffViewerCursorLineStyleIsBlendedRGB(t *testing.T) {
	viewer := &diffViewer{}
	viewer.ensureColorScheme()
	base := vaxis.Style{
		Foreground: vaxis.RGBColor(1, 2, 3),
		Background: viewer.scheme.Code,
	}

	got := viewer.rowStyle(base, true)
	want := blendRGB(base.Background, viewer.scheme.Foreground, cursorLineBlend)
	if got.Background != want {
		t.Fatalf("cursor line background = %v, want %v", got.Background, want)
	}
	if params := got.Background.Params(); len(params) != 3 {
		t.Fatalf("cursor line background params = %v, want RGB params", params)
	}
	if got.Foreground != base.Foreground {
		t.Fatalf("cursor line foreground = %v, want %v", got.Foreground, base.Foreground)
	}
}

func TestDiffViewerCursorLineSegmentsDoNotMutateCachedSegments(t *testing.T) {
	viewer := &diffViewer{}
	viewer.ensureColorScheme()
	segments := []vaxis.Segment{{
		Text: "hello",
		Style: vaxis.Style{
			Foreground: vaxis.RGBColor(1, 2, 3),
			Background: viewer.scheme.Code,
		},
	}}

	styled := viewer.rowSegments(segments, true)
	if styled[0].Style.Background == segments[0].Style.Background {
		t.Fatalf("styled background = %v, want cursor line background", styled[0].Style.Background)
	}
	if segments[0].Style.Background != viewer.scheme.Code {
		t.Fatalf("cached segment background mutated to %v", segments[0].Style.Background)
	}
}

func TestDiffViewerCursorCellUsesReverseStyle(t *testing.T) {
	viewer := &diffViewer{
		rows:   []diff.Row{{Kind: diff.RowContext, Text: "abc"}},
		cursor: selectionPoint{Row: 0, Col: 1},
	}
	viewer.ensureColorScheme()

	style := viewer.cursorCellStyle()
	wantBase := viewer.rowStyle(viewer.baseStyle(), true)
	if style.Foreground != wantBase.Background || style.Background != wantBase.Foreground {
		t.Fatalf("cursor style = %+v, want reversed base style %+v", style, wantBase)
	}
}

func TestDiffViewerCursorCellUsesRenderedCodeSegmentStyle(t *testing.T) {
	segmentStyle := vaxis.Style{
		Foreground: vaxis.RGBColor(1, 2, 3),
		Background: vaxis.RGBColor(4, 5, 6),
	}
	viewer := &diffViewer{
		rows: []diff.Row{{
			Kind:   diff.RowAdd,
			Gutter: "1 1 + ",
			Code:   "abc",
		}},
		codeSegments: [][]vaxis.Segment{{
			{Text: "abc", Style: segmentStyle},
		}},
	}
	viewer.rows[0].Text = viewer.rows[0].Gutter + viewer.rows[0].Code
	viewer.cursor = selectionPoint{Row: 0, Col: testCodeOffset(viewer.rows[0]) + 1}
	viewer.ensureColorScheme()

	style := viewer.cursorCellStyle()
	wantBase := viewer.rowStyle(segmentStyle, true)
	if style.Foreground != wantBase.Background || style.Background != wantBase.Foreground {
		t.Fatalf("cursor style = %+v, want reversed segment style %+v", style, wantBase)
	}
}

func TestCharacterAtCellReturnsSpaceForEmptyText(t *testing.T) {
	char := characterAtCell("", 0)
	if char.Grapheme != " " || char.Width != 1 {
		t.Fatalf("character = %+v, want space", char)
	}
}

func TestDiffViewerCachesRenderedCodeSegments(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{
			{
				Kind:     diff.RowAdd,
				Code:     "hello",
				FileName: "example.txt",
				InlineSpans: []diff.InlineSpan{
					{Start: 1, End: 4, Kind: diff.InlineChange},
				},
			},
		},
		highlighter: NewSyntaxHighlighter(),
	}
	viewer.ensureColorScheme()

	viewer.ensureRenderCache()

	if len(viewer.codeSegments) != 1 {
		t.Fatalf("cached rows = %d, want 1", len(viewer.codeSegments))
	}
	segments := viewer.codeSegments[0]
	if len(segments) != 3 {
		t.Fatalf("segments = %+v, want 3", segments)
	}
	if segments[0].Text != "h" || segments[0].Style.Background == viewer.scheme.AddInline {
		t.Fatalf("first segment = %+v", segments[0])
	}
	if segments[1].Text != "ell" || segments[1].Style.Background != viewer.scheme.AddInline {
		t.Fatalf("inline segment = %+v, want add inline background", segments[1])
	}
	if segments[2].Text != "o" || segments[2].Style.Background == viewer.scheme.AddInline {
		t.Fatalf("last segment = %+v", segments[2])
	}

	cached := viewer.codeSegments
	viewer.ensureRenderCache()
	if &viewer.codeSegments[0][0] != &cached[0][0] {
		t.Fatal("render cache rebuilt when rows did not change")
	}
}

func TestDiffViewerInvalidatesRenderCacheWhenTerminalColorsChange(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{
			{Kind: diff.RowAdd, Code: "hello", FileName: "example.txt"},
		},
		highlighter: NewSyntaxHighlighter(),
	}
	viewer.ensureColorScheme()
	viewer.ensureRenderCache()

	viewer.SetTerminalColors(TerminalColors{Green: vaxis.RGBColor(1, 2, 3)})

	if viewer.codeSegments != nil {
		t.Fatalf("code segment cache = %+v, want nil", viewer.codeSegments)
	}
}

func TestDiffViewerUsesChangedGutterForegroundForChangedLines(t *testing.T) {
	viewer := &diffViewer{}
	viewer.ensureColorScheme()

	addGutter := viewer.gutterStyle(diff.RowAdd)
	if addGutter.Foreground != viewer.scheme.Add {
		t.Fatalf("add gutter foreground = %v, want %v", addGutter.Foreground, viewer.scheme.Add)
	}
	if addGutter.Background != viewer.scheme.Gutter {
		t.Fatalf("add gutter background = %v, want %v", addGutter.Background, viewer.scheme.Gutter)
	}

	deleteGutter := viewer.gutterStyle(diff.RowDelete)
	if deleteGutter.Foreground != viewer.scheme.Delete {
		t.Fatalf("delete gutter foreground = %v, want %v", deleteGutter.Foreground, viewer.scheme.Delete)
	}
	if deleteGutter.Background != viewer.scheme.Gutter {
		t.Fatalf("delete gutter background = %v, want %v", deleteGutter.Background, viewer.scheme.Gutter)
	}
}

func TestDiffViewerUsesSingleDarkGutterSegment(t *testing.T) {
	viewer := &diffViewer{}
	viewer.ensureColorScheme()
	segments := viewer.gutterSegments(diff.Row{
		Kind:   diff.RowAdd,
		Gutter: "1 2 ",
		Marker: "+",
	})

	if len(segments) != 1 {
		t.Fatalf("segments = %+v, want one", segments)
	}
	if segments[0].Text != "1 2 +" || segments[0].Style != viewer.gutterStyle(diff.RowAdd) {
		t.Fatalf("gutter segment = %+v", segments[0])
	}
}

func TestDiffViewerStylesCommitPreambleRows(t *testing.T) {
	viewer := &diffViewer{}
	viewer.ensureColorScheme()

	header, ok := viewer.structuredSegments(diff.Row{Kind: diff.RowCommitHeader, Prefix: "commit ", Code: "abc123"})
	if !ok {
		t.Fatal("commit header segments missing")
	}
	if header[0].Style.Foreground != viewer.scheme.Dim || header[1].Style.Foreground != viewer.scheme.Yellow {
		t.Fatalf("header segments = %+v", header)
	}

	trailer, ok := viewer.structuredSegments(diff.Row{Kind: diff.RowCommitTrailer, Prefix: "    Reviewed-by: ", Code: "Tim"})
	if !ok {
		t.Fatal("trailer segments missing")
	}
	if trailer[0].Style.Foreground != viewer.scheme.Blue || trailer[1].Style.Foreground != viewer.scheme.Dim {
		t.Fatalf("trailer segments = %+v", trailer)
	}
}

func TestDiffViewerUsesReviewMarkerInGutterSpace(t *testing.T) {
	anchor := review.Anchor{Path: "main.go", Line: 2, Side: review.SideRight}
	viewer := &diffViewer{
		reviewDrafts: []review.CommentDraft{{
			Path: "main.go",
			Line: 2,
			Side: review.SideRight,
			Body: "comment",
		}},
	}
	viewer.ensureColorScheme()
	segments := viewer.gutterSegments(diff.Row{
		Kind:   diff.RowAdd,
		Gutter: "1 2 + ",
		Review: anchor,
	})

	if len(segments) != 2 {
		t.Fatalf("segments = %+v, want two", segments)
	}
	if got, want := segments[0].Text, "1 2 +"; got != want {
		t.Fatalf("gutter text = %q, want %q", got, want)
	}
	if got, want := segments[1].Text, "▐"; got != want {
		t.Fatalf("marker text = %q, want %q", got, want)
	}
	if got, want := segments[1].Style.Foreground, viewer.scheme.Yellow; got != want {
		t.Fatalf("marker foreground = %v, want %v", got, want)
	}
}

func TestDiffViewerStickyFileHeader(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{
			{Kind: diff.RowFile, Text: "one.go"},
			{Kind: diff.RowHunk, Text: "@@ -1 +1 @@"},
			{Kind: diff.RowDelete, Text: "-old"},
			{Kind: diff.RowBlank},
			{Kind: diff.RowFile, Text: "two.go"},
			{Kind: diff.RowHunk, Text: "@@ -1 +1 @@"},
		},
	}

	viewer.scroll = 0
	if row, ok := viewer.stickyFileHeader(); ok {
		t.Fatalf("sticky row at file header = %+v, want none", row)
	}

	viewer.scroll = 2
	row, ok := viewer.stickyFileHeader()
	if !ok || row.Text != "one.go" {
		t.Fatalf("sticky row = %+v, %v; want one.go", row, ok)
	}

	viewer.scroll = 4
	if row, ok := viewer.stickyFileHeader(); ok {
		t.Fatalf("sticky row at second file header = %+v, want none", row)
	}

	viewer.scroll = 5
	row, ok = viewer.stickyFileHeader()
	if !ok || row.Text != "two.go" {
		t.Fatalf("sticky row = %+v, %v; want two.go", row, ok)
	}
}

func TestDiffViewerModeLabels(t *testing.T) {
	viewer := &diffViewer{}
	if got := viewer.modeLabel(); got != "NORMAL" {
		t.Fatalf("mode label = %q, want NORMAL", got)
	}
	viewer.mode = modeVisual
	if got := viewer.modeLabel(); got != "VISUAL" {
		t.Fatalf("mode label = %q, want VISUAL", got)
	}
	viewer.mode = modeVisualLine
	if got := viewer.modeLabel(); got != "V-LINE" {
		t.Fatalf("mode label = %q, want V-LINE", got)
	}
	viewer.mode = modeInsert
	if got := viewer.modeLabel(); got != "INSERT" {
		t.Fatalf("mode label = %q, want INSERT", got)
	}
	viewer.mode = modeCommand
	if got := viewer.modeLabel(); got != "COMMAND" {
		t.Fatalf("mode label = %q, want COMMAND", got)
	}
	viewer.mode = modeSearch
	if got := viewer.modeLabel(); got != "SEARCH" {
		t.Fatalf("mode label = %q, want SEARCH", got)
	}
}

func TestDiffViewerStatusColorsFollowMode(t *testing.T) {
	viewer := &diffViewer{}
	viewer.ensureColorScheme()

	if got, want := viewer.statusColor(), viewer.scheme.Base.Blue; got != want {
		t.Fatalf("normal status color = %v, want blue %v", got, want)
	}
	if got, want := viewer.statusStyle().Background, viewer.scheme.Base.Blue; got != want {
		t.Fatalf("normal status background = %v, want blue %v", got, want)
	}

	viewer.mode = modeVisual
	if got, want := viewer.statusColor(), viewer.scheme.Base.Magenta; got != want {
		t.Fatalf("visual status color = %v, want magenta %v", got, want)
	}
	if got, want := viewer.statusSeparatorStyle().Foreground, viewer.scheme.Base.Magenta; got != want {
		t.Fatalf("visual separator foreground = %v, want magenta %v", got, want)
	}

	viewer.mode = modeVisualLine
	if got, want := viewer.statusColor(), viewer.scheme.Base.Magenta; got != want {
		t.Fatalf("visual line status color = %v, want magenta %v", got, want)
	}

	viewer.mode = modeInsert
	if got, want := viewer.statusColor(), viewer.scheme.Base.Green; got != want {
		t.Fatalf("insert status color = %v, want green %v", got, want)
	}
}

func TestDiffViewerEscReturnsToNormalMode(t *testing.T) {
	viewer := newTestDiffViewer(10, 10)
	viewer.mode = modeVisual
	viewer.selection = textSelection{
		Active: true,
		Anchor: selectionPoint{Row: 0, Col: 0},
		Cursor: selectionPoint{Row: 1, Col: 0},
	}

	cmd, err := viewer.HandleEvent(vaxis.Key{Text: "\x1b", Keycode: vaxis.KeyEsc})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
	}
	if viewer.mode != modeNormal {
		t.Fatalf("mode = %v, want normal", viewer.mode)
	}
	if viewer.selection.Active {
		t.Fatalf("selection still active: %+v", viewer.selection)
	}
}

func TestDiffViewerVisualModeKeys(t *testing.T) {
	tests := []struct {
		name string
		key  vaxis.Key
		mode viewMode
	}{
		{
			name: "v enters visual",
			key:  vaxis.Key{Text: "v", Keycode: 'v'},
			mode: modeVisual,
		},
		{
			name: "V enters visual line",
			key:  vaxis.Key{Text: "V", Keycode: 'V'},
			mode: modeVisualLine,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viewer := newTestDiffViewer(10, 10)
			viewer.rows[0] = diff.Row{
				Kind:   diff.RowAdd,
				Gutter: "1 1 + ",
				Code:   "hello",
			}
			viewer.rows[0].Text = viewer.rows[0].Gutter + viewer.rows[0].Code

			cmd, err := viewer.HandleEvent(tt.key)
			if err != nil {
				t.Fatal(err)
			}
			if cmd != CommandRedraw {
				t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
			}
			if viewer.mode != tt.mode {
				t.Fatalf("mode = %v, want %v", viewer.mode, tt.mode)
			}
			if !viewer.selection.Active {
				t.Fatal("selection inactive")
			}
		})
	}
}

func TestDiffViewerVisualLineSelectsWholeCodeRows(t *testing.T) {
	rows := []diff.Row{
		{
			Kind:   diff.RowAdd,
			Gutter: "1 1 + ",
			Code:   "hello",
		},
		{
			Kind:   diff.RowAdd,
			Gutter: "2 2 + ",
			Code:   "world",
		},
	}
	for i := range rows {
		rows[i].Text = rows[i].Gutter + rows[i].Code
	}
	viewer := &diffViewer{rows: rows}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	cmd, err := viewer.HandleEvent(vaxis.Key{Text: "V", Keycode: 'V'})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
	}
	viewer.moveCursorRows(1)

	if got, want := viewer.ClipboardText(), "hello\nworld"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
}

func TestDiffViewerVisualLineSelectsWholeCodeRowsWhenMovingUp(t *testing.T) {
	rows := []diff.Row{
		{Kind: diff.RowAdd, Gutter: "1 1 + ", Code: "hello"},
		{Kind: diff.RowAdd, Gutter: "2 2 + ", Code: "world"},
	}
	for i := range rows {
		rows[i].Text = rows[i].Gutter + rows[i].Code
	}
	viewer := &diffViewer{rows: rows, cursor: selectionPoint{Row: 1, Col: testCodeOffset(rows[1])}}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))
	viewer.cursorGoal = viewer.cursor.Col

	cmd, err := viewer.HandleEvent(vaxis.Key{Text: "V", Keycode: 'V'})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
	}
	viewer.moveCursorRows(-1)

	if got, want := viewer.ClipboardText(), "hello\nworld"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
}

func TestDiffViewerInsertCreatesReviewDraftAtCursor(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{
			Kind:   diff.RowAdd,
			Text:   "hello",
			Code:   "hello",
			Review: review.Anchor{Path: "main.go", Line: 12, Side: review.SideRight, CommitID: "abc123"},
		}},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	cmd := openReviewCommentEditor(t, viewer)
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
	}
	if len(viewer.reviewDrafts) != 0 {
		t.Fatalf("draft count after open = %d, want 0", len(viewer.reviewDrafts))
	}
	cmd = submitReviewComment(t, viewer, "looks good")
	if cmd != CommandRedraw {
		t.Fatalf("submit command = %v, want %v", cmd, CommandRedraw)
	}
	if len(viewer.reviewDrafts) != 1 {
		t.Fatalf("draft count = %d, want 1", len(viewer.reviewDrafts))
	}
	want := review.CommentDraft{Path: "main.go", Line: 12, Side: review.SideRight, CommitID: "abc123", Body: "looks good"}
	if got := viewer.reviewDrafts[0]; got != want {
		t.Fatalf("draft = %+v, want %+v", got, want)
	}
	if !viewer.hasReviewDraft(viewer.rows[0].Review) {
		t.Fatal("draft marker lookup failed")
	}
}

func TestDiffViewerInsertCreatesReviewDraftForSelectionRange(t *testing.T) {
	rows := []diff.Row{
		{Kind: diff.RowAdd, Text: "hello", Code: "hello", Review: review.Anchor{Path: "main.go", Line: 12, Side: review.SideRight}},
		{Kind: diff.RowAdd, Text: "world", Code: "world", Review: review.Anchor{Path: "main.go", Line: 13, Side: review.SideRight}},
	}
	viewer := &diffViewer{
		rows: rows,
		selection: textSelection{
			Active: true,
			Anchor: selectionPoint{Row: 0, Col: 0},
			Cursor: selectionPoint{Row: 1, Col: 0},
		},
		mode: modeVisual,
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	cmd := openReviewCommentEditor(t, viewer)
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
	}
	cmd = submitReviewComment(t, viewer, "range comment")
	if cmd != CommandRedraw {
		t.Fatalf("submit command = %v, want %v", cmd, CommandRedraw)
	}
	if len(viewer.reviewDrafts) != 1 {
		t.Fatalf("draft count = %d, want 1", len(viewer.reviewDrafts))
	}
	want := review.CommentDraft{
		Path:      "main.go",
		Body:      "range comment",
		StartLine: 12,
		StartSide: review.SideRight,
		Line:      13,
		Side:      review.SideRight,
	}
	if got := viewer.reviewDrafts[0]; got != want {
		t.Fatalf("draft = %+v, want %+v", got, want)
	}
	if viewer.selection.Active || viewer.mode != modeNormal {
		t.Fatalf("selection/mode after draft = active:%v mode:%v, want normal", viewer.selection.Active, viewer.mode)
	}
	if !viewer.hasReviewDraft(rows[0].Review) || !viewer.hasReviewDraft(rows[1].Review) {
		t.Fatal("draft range marker lookup failed")
	}
}

func TestDiffViewerInsertCreatesReviewDraftWithColumnsForSingleLineVisualSelection(t *testing.T) {
	row := diff.Row{
		Kind:   diff.RowAdd,
		Gutter: "1 1 + ",
		Code:   "hello",
		Review: review.Anchor{Path: "main.go", Line: 12, Side: review.SideRight},
	}
	row.Text = row.Gutter + row.Code
	codeOffset := testCodeOffset(row)
	viewer := &diffViewer{
		rows: []diff.Row{row},
		selection: textSelection{
			Active: true,
			Anchor: selectionPoint{Row: 0, Col: codeOffset + 1},
			Cursor: selectionPoint{Row: 0, Col: codeOffset + 3},
		},
		mode: modeVisual,
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	cmd := openReviewCommentEditor(t, viewer)
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
	}
	cmd = submitReviewComment(t, viewer, "column comment")
	if cmd != CommandRedraw {
		t.Fatalf("submit command = %v, want %v", cmd, CommandRedraw)
	}
	if len(viewer.reviewDrafts) != 1 {
		t.Fatalf("draft count = %d, want 1", len(viewer.reviewDrafts))
	}
	draft := viewer.reviewDrafts[0]
	if draft.StartColumn == nil || draft.EndColumn == nil {
		t.Fatalf("draft columns missing: %+v", draft)
	}
	if got, want := *draft.StartColumn, 2; got != want {
		t.Fatalf("start column = %d, want %d", got, want)
	}
	if got, want := *draft.EndColumn, 4; got != want {
		t.Fatalf("end column = %d, want %d", got, want)
	}
}

func TestDiffViewerInsertDoesNotAddColumnsForVisualLineSelection(t *testing.T) {
	row := diff.Row{
		Kind:   diff.RowAdd,
		Gutter: "1 1 + ",
		Code:   "hello",
		Review: review.Anchor{Path: "main.go", Line: 12, Side: review.SideRight},
	}
	row.Text = row.Gutter + row.Code
	codeOffset := testCodeOffset(row)
	viewer := &diffViewer{
		rows: []diff.Row{row},
		selection: textSelection{
			Active: true,
			Anchor: selectionPoint{Row: 0, Col: codeOffset},
			Cursor: selectionPoint{Row: 0, Col: codeOffset + textCellWidth(row.Code) - 1},
		},
		mode: modeVisualLine,
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	cmd := openReviewCommentEditor(t, viewer)
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
	}
	cmd = submitReviewComment(t, viewer, "line comment")
	if cmd != CommandRedraw {
		t.Fatalf("submit command = %v, want %v", cmd, CommandRedraw)
	}
	draft := viewer.reviewDrafts[0]
	if draft.StartColumn != nil || draft.EndColumn != nil {
		t.Fatalf("draft columns = %v:%v, want nil", draft.StartColumn, draft.EndColumn)
	}
}

func TestDiffViewerCommentEditorSupportsMultipleLines(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{
			Kind:   diff.RowAdd,
			Text:   "hello",
			Code:   "hello",
			Review: review.Anchor{Path: "main.go", Line: 12, Side: review.SideRight},
		}},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))
	openReviewCommentEditor(t, viewer)

	if cmd, err := viewer.HandleEvent(vaxis.Key{Text: "first"}); err != nil {
		t.Fatal(err)
	} else if cmd != CommandRedraw {
		t.Fatalf("text command = %v, want %v", cmd, CommandRedraw)
	}
	if cmd, err := viewer.HandleEvent(vaxis.Key{Keycode: vaxis.KeyEnter}); err != nil {
		t.Fatal(err)
	} else if cmd != CommandRedraw {
		t.Fatalf("enter command = %v, want %v", cmd, CommandRedraw)
	}
	if cmd := submitReviewComment(t, viewer, "second"); cmd != CommandRedraw {
		t.Fatalf("submit command = %v, want %v", cmd, CommandRedraw)
	}

	if got, want := viewer.reviewDrafts[0].Body, "first\nsecond"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestDiffViewerCommentEditorOpensBelowCursor(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{
			Kind:   diff.RowAdd,
			Text:   "hello",
			Code:   "hello",
			Review: review.Anchor{Path: "main.go", Line: 12, Side: review.SideRight},
		}},
		cursor: selectionPoint{Row: 0, Col: 2},
	}
	viewer.Layout(Tight(Size{Width: 40, Height: 10}))
	openReviewCommentEditor(t, viewer)

	x, y, _, _, ok := viewer.commentEditorRect(40, 10)
	if !ok {
		t.Fatal("comment editor rect missing")
	}
	if x != 2 || y != 1 {
		t.Fatalf("editor origin = %d,%d, want 2,1", x, y)
	}
}

func TestDiffViewerCommentEditorScrollsIntoView(t *testing.T) {
	rows := make([]diff.Row, 20)
	for i := range rows {
		rows[i] = diff.Row{
			Kind:   diff.RowAdd,
			Text:   "hello",
			Code:   "hello",
			Review: review.Anchor{Path: "main.go", Line: i + 1, Side: review.SideRight},
		}
	}
	viewer := &diffViewer{
		rows:   rows,
		cursor: selectionPoint{Row: 19, Col: 0},
	}
	viewer.Layout(Tight(Size{Width: 40, Height: 10}))
	openReviewCommentEditor(t, viewer)

	_, y, _, height, ok := viewer.commentEditorRect(40, 10)
	if !ok {
		t.Fatal("comment editor rect missing")
	}
	visible := viewer.visibleRowCapacity()
	if y+height > visible {
		t.Fatalf("editor bottom = %d, visible rows = %d", y+height, visible)
	}
	if viewer.scroll <= 11 {
		t.Fatalf("scroll = %d, want extra bottom padding beyond normal max", viewer.scroll)
	}
}

func TestDiffViewerCommentEditorEscapeClosesAndSaves(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{
			{
				Kind:   diff.RowAdd,
				Text:   "hello",
				Code:   "hello",
				Review: review.Anchor{Path: "main.go", Line: 12, Side: review.SideRight},
			},
			{
				Kind:   diff.RowAdd,
				Text:   "world",
				Code:   "world",
				Review: review.Anchor{Path: "main.go", Line: 13, Side: review.SideRight},
			},
		},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))
	openReviewCommentEditor(t, viewer)
	if cmd, err := viewer.HandleEvent(vaxis.Key{Text: "saved"}); err != nil {
		t.Fatal(err)
	} else if cmd != CommandRedraw {
		t.Fatalf("text command = %v, want %v", cmd, CommandRedraw)
	}

	cmd, err := viewer.HandleEvent(vaxis.Key{Keycode: vaxis.KeyEsc})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
	}
	if viewer.editor != nil {
		t.Fatal("editor still open after escape")
	}
	if viewer.mode != modeNormal {
		t.Fatalf("mode = %v, want normal", viewer.mode)
	}
	if len(viewer.reviewDrafts) != 1 {
		t.Fatalf("draft count = %d, want 1", len(viewer.reviewDrafts))
	}
	if got, want := viewer.reviewDrafts[0].Body, "saved"; got != want {
		t.Fatalf("draft body = %q, want %q", got, want)
	}

	cmd, err = viewer.HandleEvent(vaxis.Key{Text: "j", Keycode: 'j'})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("move command = %v, want %v", cmd, CommandRedraw)
	}
	if viewer.cursor.Row != 1 {
		t.Fatalf("cursor row = %d, want 1", viewer.cursor.Row)
	}
}

func TestDiffViewerInsertReopensExistingComment(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{
			Kind:   diff.RowAdd,
			Text:   "hello",
			Code:   "hello",
			Review: review.Anchor{Path: "main.go", Line: 12, Side: review.SideRight},
		}},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))
	openReviewCommentEditor(t, viewer)
	submitReviewComment(t, viewer, "first")

	cmd, err := viewer.HandleEvent(vaxis.Key{Text: "i", Keycode: 'i'})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("insert command = %v, want %v", cmd, CommandRedraw)
	}
	if viewer.mode != modeInsert {
		t.Fatalf("mode = %v, want insert", viewer.mode)
	}
	if viewer.editor == nil {
		t.Fatal("editor is nil")
	}
	if got, want := viewer.editor.body(), "first"; got != want {
		t.Fatalf("editor body = %q, want %q", got, want)
	}
	if viewer.editor.draftIndex != 0 {
		t.Fatalf("draft index = %d, want 0", viewer.editor.draftIndex)
	}
	if _, err := viewer.HandleEvent(vaxis.Key{Text: " updated"}); err != nil {
		t.Fatal(err)
	}
	submitReviewComment(t, viewer, "")
	if len(viewer.reviewDrafts) != 1 {
		t.Fatalf("draft count = %d, want 1", len(viewer.reviewDrafts))
	}
	if got, want := viewer.reviewDrafts[0].Body, "first updated"; got != want {
		t.Fatalf("draft body = %q, want %q", got, want)
	}
}

func TestDiffViewerCommandQQuits(t *testing.T) {
	viewer := newTestDiffViewer(1, 10)

	cmd := executeCommand(t, viewer, "q")
	if cmd != CommandQuit {
		t.Fatalf("command = %v, want quit", cmd)
	}
}

func TestDiffViewerCommandQWarnsWithUnsavedComments(t *testing.T) {
	viewer := newTestDiffViewer(1, 10)
	viewer.reviewDrafts = []review.CommentDraft{{Path: "main.go", Line: 1, Side: review.SideRight, Body: "comment"}}
	viewer.reviewDirty = true

	cmd := executeCommand(t, viewer, "q")
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want redraw", cmd)
	}
	if viewer.mode != modeNormal {
		t.Fatalf("mode = %v, want normal", viewer.mode)
	}
	if viewer.statusMessage == "" {
		t.Fatal("status message is empty")
	}
}

func TestDiffViewerCommandWSavesComments(t *testing.T) {
	viewer := newTestDiffViewer(1, 10)
	viewer.reviewDrafts = []review.CommentDraft{{Path: "main.go", Line: 1, Side: review.SideRight, Body: "comment"}}
	viewer.reviewDirty = true

	cmd := executeCommand(t, viewer, "w")
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want redraw", cmd)
	}
	if viewer.reviewDirty {
		t.Fatal("review dirty after :w")
	}
	if got, want := viewer.statusMessage, "Comments saved."; got != want {
		t.Fatalf("status message = %q, want %q", got, want)
	}
}

func TestDiffViewerCommandWWritesCommentsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".comview", "comments.json")
	viewer := newTestDiffViewer(1, 10)
	viewer.reviewFile = path
	viewer.reviewDrafts = []review.CommentDraft{{Path: "main.go", Line: 1, Side: review.SideRight, Body: "comment"}}
	viewer.reviewDirty = true

	cmd := executeCommand(t, viewer, "w")
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want redraw", cmd)
	}
	file, err := review.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Comments) != 1 || file.Comments[0].Body != "comment" {
		t.Fatalf("comments file = %+v", file)
	}
}

func TestDiffViewerCommandWQWritesAndQuits(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".comview", "comments.json")
	viewer := newTestDiffViewer(1, 10)
	viewer.reviewFile = path
	viewer.reviewDrafts = []review.CommentDraft{{Path: "main.go", Line: 1, Side: review.SideRight, Body: "comment"}}
	viewer.reviewDirty = true

	cmd := executeCommand(t, viewer, "wq")
	if cmd != CommandQuit {
		t.Fatalf("command = %v, want quit", cmd)
	}
	if viewer.reviewDirty {
		t.Fatal("review dirty after :wq")
	}
	file, err := review.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Comments) != 1 || file.Comments[0].Body != "comment" {
		t.Fatalf("comments file = %+v", file)
	}
}

func TestDiffViewerCommandQBangQuitsWithUnsavedComments(t *testing.T) {
	viewer := newTestDiffViewer(1, 10)
	viewer.reviewDirty = true

	cmd := executeCommand(t, viewer, "q!")
	if cmd != CommandQuit {
		t.Fatalf("command = %v, want quit", cmd)
	}
}

func TestDiffViewerCommandCursorUsesStatusBar(t *testing.T) {
	viewer := &diffViewer{
		mode:        modeCommand,
		commandLine: "w",
	}

	col, row, ok := viewer.commandCursorPositionForSize(20, 10)
	if !ok {
		t.Fatal("command cursor position missing")
	}
	if col != 2 || row != 9 {
		t.Fatalf("command cursor = %d,%d, want 2,9", col, row)
	}
}

func TestDiffViewerSlashSearchMovesToMatch(t *testing.T) {
	rows := []diff.Row{
		{Kind: diff.RowContext, Gutter: "1 1   ", Code: "alpha"},
		{Kind: diff.RowContext, Gutter: "2 2   ", Code: "needle"},
	}
	for i := range rows {
		rows[i].Text = rows[i].Gutter + rows[i].Code
	}
	viewer := &diffViewer{rows: rows}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	cmd, err := viewer.HandleEvent(vaxis.Key{Text: "/", Keycode: '/'})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw || viewer.mode != modeSearch {
		t.Fatalf("slash command = %v mode=%v, want redraw search", cmd, viewer.mode)
	}
	if cmd, err = viewer.HandleEvent(vaxis.Key{Text: "needle"}); err != nil {
		t.Fatal(err)
	} else if cmd != CommandRedraw {
		t.Fatalf("query command = %v, want redraw", cmd)
	}
	if cmd, err = viewer.HandleEvent(vaxis.Key{Keycode: vaxis.KeyEnter}); err != nil {
		t.Fatal(err)
	} else if cmd != CommandRedraw {
		t.Fatalf("enter command = %v, want redraw", cmd)
	}

	if viewer.mode != modeNormal {
		t.Fatalf("mode = %v, want normal", viewer.mode)
	}
	if got, want := viewer.cursor.Row, 1; got != want {
		t.Fatalf("cursor row = %d, want %d", got, want)
	}
	if got, want := viewer.cursor.Col, testCodeOffset(rows[1]); got != want {
		t.Fatalf("cursor col = %d, want %d", got, want)
	}
}

func TestDiffViewerSearchNextPrevious(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{
			{Kind: diff.RowContext, Text: "one needle"},
			{Kind: diff.RowContext, Text: "two needle"},
		},
		searchQuery: "needle",
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))
	viewer.updateSearchMatches()

	if !viewer.moveSearchMatch(1) {
		t.Fatal("next search failed")
	}
	if got, want := viewer.cursor.Row, 0; got != want {
		t.Fatalf("cursor row = %d, want %d", got, want)
	}
	if !viewer.moveSearchMatch(1) {
		t.Fatal("second next search failed")
	}
	if got, want := viewer.cursor.Row, 1; got != want {
		t.Fatalf("cursor row = %d, want %d", got, want)
	}
	if !viewer.moveSearchMatch(-1) {
		t.Fatal("previous search failed")
	}
	if got, want := viewer.cursor.Row, 0; got != want {
		t.Fatalf("cursor row = %d, want %d", got, want)
	}
}

func TestDiffViewerEscapeClearsSearch(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{Kind: diff.RowContext, Text: "needle"}},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))
	viewer.searchQuery = "needle"
	viewer.updateSearchMatches()
	viewer.searchIndex = 0

	cmd, err := viewer.HandleEvent(vaxis.Key{Keycode: vaxis.KeyEsc})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want redraw", cmd)
	}
	if viewer.searchQuery != "" || len(viewer.searchMatches) != 0 || viewer.searchIndex != -1 {
		t.Fatalf("search state = query:%q matches:%+v index:%d", viewer.searchQuery, viewer.searchMatches, viewer.searchIndex)
	}
}

func TestDiffViewerSearchSegmentsHighlightMatches(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{Kind: diff.RowContext, Text: "one needle"}},
		searchMatches: []searchMatch{{
			Row:   0,
			Start: 4,
			End:   10,
		}},
	}
	viewer.ensureColorScheme()

	segments := viewer.searchSegments(0, viewer.rows[0], []vaxis.Segment{{Text: "one needle", Style: viewer.baseStyle()}})

	if len(segments) != 2 {
		t.Fatalf("segments = %+v, want 2", segments)
	}
	if got, want := segments[1].Text, "needle"; got != want {
		t.Fatalf("highlight text = %q, want %q", got, want)
	}
	if segments[1].Style.Background != viewer.scheme.Yellow {
		t.Fatalf("highlight style = %+v", segments[1].Style)
	}
}

func TestDiffViewerPlainQDoesNotQuit(t *testing.T) {
	viewer := newTestDiffViewer(1, 10)

	cmd, err := viewer.HandleEvent(vaxis.Key{Text: "q", Keycode: 'q'})
	if err != nil {
		t.Fatal(err)
	}
	if cmd == CommandQuit {
		t.Fatal("plain q quit, want no quit")
	}
}

func TestDiffViewerVimNavigationKeys(t *testing.T) {
	tests := []struct {
		name       string
		start      int
		cursor     int
		key        vaxis.Key
		wantScroll int
		wantCursor int
		wantCmd    Command
		pending    string
		wantPend   string
	}{
		{
			name:       "G moves cursor to bottom",
			key:        vaxis.Key{Text: "G", Keycode: 'G'},
			wantScroll: 91,
			wantCursor: 99,
			wantCmd:    CommandRedraw,
		},
		{
			name:       "End moves cursor to bottom",
			key:        vaxis.Key{Keycode: vaxis.KeyEnd},
			wantScroll: 91,
			wantCursor: 99,
			wantCmd:    CommandRedraw,
		},
		{
			name:       "Home moves cursor to top",
			start:      40,
			cursor:     40,
			key:        vaxis.Key{Keycode: vaxis.KeyHome},
			wantScroll: 0,
			wantCursor: 0,
			wantCmd:    CommandRedraw,
		},
		{
			name:       "j moves cursor down",
			start:      10,
			cursor:     10,
			key:        vaxis.Key{Text: "j", Keycode: 'j'},
			wantScroll: 10,
			wantCursor: 11,
			wantCmd:    CommandRedraw,
		},
		{
			name:       "Down moves cursor down",
			start:      10,
			cursor:     10,
			key:        vaxis.Key{Keycode: vaxis.KeyDown},
			wantScroll: 10,
			wantCursor: 11,
			wantCmd:    CommandRedraw,
		},
		{
			name:       "k moves cursor up",
			start:      10,
			cursor:     10,
			key:        vaxis.Key{Text: "k", Keycode: 'k'},
			wantScroll: 9,
			wantCursor: 9,
			wantCmd:    CommandRedraw,
		},
		{
			name:       "Up moves cursor up",
			start:      10,
			cursor:     10,
			key:        vaxis.Key{Keycode: vaxis.KeyUp},
			wantScroll: 9,
			wantCursor: 9,
			wantCmd:    CommandRedraw,
		},
		{
			name:       "Ctrl+d moves cursor down half page",
			start:      10,
			cursor:     10,
			key:        vaxis.Key{Keycode: 'd', Modifiers: vaxis.ModCtrl},
			wantScroll: 10,
			wantCursor: 14,
			wantCmd:    CommandRedraw,
		},
		{
			name:       "Page Down moves cursor down half page",
			start:      10,
			cursor:     10,
			key:        vaxis.Key{Keycode: vaxis.KeyPgDown},
			wantScroll: 10,
			wantCursor: 14,
			wantCmd:    CommandRedraw,
		},
		{
			name:       "Ctrl+u moves cursor up half page",
			start:      10,
			cursor:     10,
			key:        vaxis.Key{Keycode: 'u', Modifiers: vaxis.ModCtrl},
			wantScroll: 6,
			wantCursor: 6,
			wantCmd:    CommandRedraw,
		},
		{
			name:       "Page Up moves cursor up half page",
			start:      10,
			cursor:     10,
			key:        vaxis.Key{Keycode: vaxis.KeyPgUp},
			wantScroll: 6,
			wantCursor: 6,
			wantCmd:    CommandRedraw,
		},
		{
			name:       "g waits for second g",
			start:      10,
			cursor:     10,
			key:        vaxis.Key{Text: "g", Keycode: 'g'},
			wantScroll: 10,
			wantCursor: 10,
			wantCmd:    CommandNone,
			wantPend:   "g",
		},
		{
			name:       "second g moves cursor to top",
			start:      10,
			cursor:     10,
			key:        vaxis.Key{Text: "g", Keycode: 'g'},
			wantScroll: 0,
			wantCursor: 0,
			wantCmd:    CommandRedraw,
			pending:    "g",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viewer := newTestDiffViewer(100, 10)
			viewer.scroll = tt.start
			viewer.cursor.Row = tt.cursor
			viewer.cursorGoal = viewer.cursor.Col
			if tt.pending != "" {
				viewer.keys.Set(tt.pending, time.Now())
			}

			cmd, err := viewer.HandleEvent(tt.key)
			if err != nil {
				t.Fatal(err)
			}
			if cmd != tt.wantCmd {
				t.Fatalf("command = %v, want %v", cmd, tt.wantCmd)
			}
			if viewer.scroll != tt.wantScroll {
				t.Fatalf("scroll = %d, want %d", viewer.scroll, tt.wantScroll)
			}
			if viewer.cursor.Row != tt.wantCursor {
				t.Fatalf("cursor row = %d, want %d", viewer.cursor.Row, tt.wantCursor)
			}
			if viewer.keys.Pending() != tt.wantPend {
				t.Fatalf("pending keys = %q, want %q", viewer.keys.Pending(), tt.wantPend)
			}
		})
	}
}

func TestDiffViewerIgnoresKeyReleaseEvents(t *testing.T) {
	viewer := newTestDiffViewer(100, 10)
	viewer.scroll = 10

	cmd, err := viewer.HandleEvent(vaxis.Key{
		Text:      "j",
		Keycode:   'j',
		EventType: vaxis.EventRelease,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandNone {
		t.Fatalf("command = %v, want %v", cmd, CommandNone)
	}
	if viewer.scroll != 10 {
		t.Fatalf("scroll = %d, want 10", viewer.scroll)
	}
}

func TestDiffViewerHorizontalNavigationKeys(t *testing.T) {
	tests := []struct {
		name       string
		start      int
		cursor     int
		key        vaxis.Key
		wantScroll int
		wantCursor int
	}{
		{
			name:       "l moves cursor right and scrolls at edge",
			cursor:     79,
			key:        vaxis.Key{Text: "l", Keycode: 'l'},
			wantScroll: 1,
			wantCursor: 80,
		},
		{
			name:       "Right moves cursor right and scrolls at edge",
			cursor:     79,
			key:        vaxis.Key{Keycode: vaxis.KeyRight},
			wantScroll: 1,
			wantCursor: 80,
		},
		{
			name:       "h moves cursor left and scrolls at edge",
			start:      5,
			cursor:     5,
			key:        vaxis.Key{Text: "h", Keycode: 'h'},
			wantScroll: 4,
			wantCursor: 4,
		},
		{
			name:       "Left moves cursor left and scrolls at edge",
			start:      5,
			cursor:     5,
			key:        vaxis.Key{Keycode: vaxis.KeyLeft},
			wantScroll: 4,
			wantCursor: 4,
		},
		{
			name:       "left clamps at zero",
			start:      0,
			cursor:     0,
			key:        vaxis.Key{Keycode: vaxis.KeyLeft},
			wantScroll: 0,
			wantCursor: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viewer := newTestDiffViewer(3, 10)
			viewer.rows[0].Kind = diff.RowContext
			viewer.rows[0].Code = strings.Repeat("x", 120)
			viewer.rows[0].Text = viewer.rows[0].Code
			viewer.xScroll = tt.start
			viewer.cursor.Col = tt.cursor
			viewer.cursorGoal = viewer.cursor.Col

			cmd, err := viewer.HandleEvent(tt.key)
			if err != nil {
				t.Fatal(err)
			}
			if cmd != CommandRedraw {
				t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
			}
			if viewer.xScroll != tt.wantScroll {
				t.Fatalf("xScroll = %d, want %d", viewer.xScroll, tt.wantScroll)
			}
			if viewer.cursor.Col != tt.wantCursor {
				t.Fatalf("cursor col = %d, want %d", viewer.cursor.Col, tt.wantCursor)
			}
		})
	}
}

func TestDiffViewerCursorDoesNotEnterGutter(t *testing.T) {
	row := diff.Row{
		Kind:   diff.RowAdd,
		Gutter: "12 34 + ",
		Code:   "abcdef",
	}
	row.Text = row.Gutter + row.Code
	codeOffset := testCodeOffset(row)

	t.Run("left clamps at code start", func(t *testing.T) {
		viewer := &diffViewer{rows: []diff.Row{row}}
		viewer.Layout(Tight(Size{Width: 80, Height: 10}))
		viewer.cursor = selectionPoint{Row: 0, Col: codeOffset}
		viewer.cursorGoal = viewer.cursor.Col

		cmd, err := viewer.HandleEvent(vaxis.Key{Keycode: vaxis.KeyLeft})
		if err != nil {
			t.Fatal(err)
		}
		if cmd != CommandRedraw {
			t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
		}
		if viewer.cursor.Col != codeOffset {
			t.Fatalf("cursor col = %d, want code offset %d", viewer.cursor.Col, codeOffset)
		}
	})

	t.Run("mouse press in gutter moves cursor to code start", func(t *testing.T) {
		viewer := &diffViewer{rows: []diff.Row{row}}
		viewer.Layout(Tight(Size{Width: 80, Height: 10}))

		cmd, err := viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventPress,
			Row:       0,
			Col:       0,
		})
		if err != nil {
			t.Fatal(err)
		}
		if cmd != CommandRedraw {
			t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
		}
		if viewer.cursor.Col != codeOffset {
			t.Fatalf("cursor col = %d, want code offset %d", viewer.cursor.Col, codeOffset)
		}
	})

	t.Run("empty code clamps to virtual code cell", func(t *testing.T) {
		empty := row
		empty.Code = ""
		empty.Text = empty.Gutter
		viewer := &diffViewer{rows: []diff.Row{empty}}
		viewer.Layout(Tight(Size{Width: 80, Height: 10}))
		viewer.cursor = selectionPoint{Row: 0, Col: 0}
		viewer.clampCursor()

		if viewer.cursor.Col != codeOffset {
			t.Fatalf("cursor col = %d, want code offset %d", viewer.cursor.Col, codeOffset)
		}
	})
}

func TestDiffViewerPendingGExpires(t *testing.T) {
	viewer := newTestDiffViewer(100, 10)
	viewer.scroll = 10
	viewer.keys.Set("g", time.Now().Add(-pendingKeyTimeout-time.Second))

	cmd, err := viewer.HandleEvent(vaxis.Key{Text: "g", Keycode: 'g'})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandNone {
		t.Fatalf("command = %v, want %v", cmd, CommandNone)
	}
	if viewer.scroll != 10 {
		t.Fatalf("scroll = %d, want 10", viewer.scroll)
	}
	if viewer.keys.Pending() != "g" {
		t.Fatalf("pending keys = %q, want %q", viewer.keys.Pending(), "g")
	}
}

func TestDiffViewerCtrlDDoesNotQuit(t *testing.T) {
	viewer := newTestDiffViewer(100, 10)

	cmd, err := viewer.HandleEvent(vaxis.Key{Keycode: 'd', Modifiers: vaxis.ModCtrl})
	if err != nil {
		t.Fatal(err)
	}
	if cmd == CommandQuit {
		t.Fatal("Ctrl+d quit, want scroll command")
	}
}

func TestDiffViewerMouseWheelScrolls(t *testing.T) {
	tests := []struct {
		name   string
		start  int
		mouse  vaxis.Mouse
		want   int
		keys   string
		noKeys bool
	}{
		{
			name:  "wheel down scrolls down",
			start: 10,
			mouse: vaxis.Mouse{Button: vaxis.MouseWheelDown},
			want:  11,
			keys:  "g",
		},
		{
			name:  "wheel up scrolls up",
			start: 10,
			mouse: vaxis.Mouse{Button: vaxis.MouseWheelUp},
			want:  9,
			keys:  "g",
		},
		{
			name:  "wheel up clamps at top",
			start: 1,
			mouse: vaxis.Mouse{Button: vaxis.MouseWheelUp},
			want:  0,
		},
		{
			name:   "non-wheel mouse does not scroll",
			start:  10,
			mouse:  vaxis.Mouse{Button: vaxis.MouseNoButton, EventType: vaxis.EventMotion},
			want:   10,
			keys:   "g",
			noKeys: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viewer := newTestDiffViewer(100, 10)
			viewer.scroll = tt.start
			if tt.keys != "" {
				viewer.keys.Set(tt.keys, time.Now())
			}

			cmd, err := viewer.HandleEvent(tt.mouse)
			if err != nil {
				t.Fatal(err)
			}
			if tt.mouse.Button == vaxis.MouseNoButton && cmd != CommandNone {
				t.Fatalf("command = %v, want %v", cmd, CommandNone)
			}
			if tt.mouse.Button != vaxis.MouseNoButton && cmd != CommandRedraw {
				t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
			}
			if viewer.scroll != tt.want {
				t.Fatalf("scroll = %d, want %d", viewer.scroll, tt.want)
			}
			if tt.noKeys {
				if viewer.keys.Pending() != tt.keys {
					t.Fatalf("pending keys = %q, want %q", viewer.keys.Pending(), tt.keys)
				}
				return
			}
			if viewer.keys.Pending() != "" {
				t.Fatalf("pending keys = %q, want empty", viewer.keys.Pending())
			}
		})
	}
}

func TestDiffViewerMouseWheelExtendsDraggingSelection(t *testing.T) {
	viewer := newTestDiffViewer(100, 10)
	for i := range viewer.rows {
		viewer.rows[i].Code = "abcdef"
		viewer.rows[i].Text = "abcdef"
	}
	viewer.scroll = 10
	viewer.selection = textSelection{
		Active:   true,
		Dragging: true,
		Anchor: selectionPoint{
			Row: 10,
			Col: 0,
		},
		Cursor: selectionPoint{
			Row: 10,
			Col: 0,
		},
	}

	cmd, err := viewer.HandleEvent(vaxis.Mouse{
		Button: vaxis.MouseWheelDown,
		Row:    5,
		Col:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
	}
	if viewer.scroll != 11 {
		t.Fatalf("scroll = %d, want 11", viewer.scroll)
	}
	if got := viewer.selection.Cursor; got != (selectionPoint{Row: 16, Col: 2}) {
		t.Fatalf("cursor = %+v, want row 16 col 2", got)
	}
}

func TestDiffViewerMouseWheelDoesNotExtendFinishedSelection(t *testing.T) {
	viewer := newTestDiffViewer(100, 10)
	for i := range viewer.rows {
		viewer.rows[i].Code = "abcdef"
		viewer.rows[i].Text = "abcdef"
	}
	viewer.scroll = 10
	viewer.selection = textSelection{
		Active: true,
		Anchor: selectionPoint{
			Row: 10,
			Col: 0,
		},
		Cursor: selectionPoint{
			Row: 10,
			Col: 0,
		},
	}

	_, err := viewer.HandleEvent(vaxis.Mouse{
		Button: vaxis.MouseWheelDown,
		Row:    5,
		Col:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := viewer.selection.Cursor; got != (selectionPoint{Row: 10, Col: 0}) {
		t.Fatalf("cursor = %+v, want unchanged", got)
	}
}

func TestDiffViewerMouseSelectionCopiesText(t *testing.T) {
	rows := []diff.Row{
		{Kind: diff.RowContext, Gutter: "1 1   ", Code: "abcde"},
		{Kind: diff.RowContext, Gutter: "2 2   ", Code: "fghij"},
	}
	for i := range rows {
		rows[i].Text = rows[i].Gutter + rows[i].Code
	}
	viewer := &diffViewer{
		rows: rows,
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))
	codeOffset := testCodeOffset(rows[0])

	cmd, err := viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventPress,
		Row:       0,
		Col:       codeOffset + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("press command = %v, want %v", cmd, CommandRedraw)
	}
	if got := viewer.cursor; got != (selectionPoint{Row: 0, Col: codeOffset + 1}) {
		t.Fatalf("cursor after press = %+v, want row 0 col %d", got, codeOffset+1)
	}

	cmd, err = viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventMotion,
		Row:       1,
		Col:       codeOffset + 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("motion command = %v, want %v", cmd, CommandRedraw)
	}
	if got := viewer.cursor; got != (selectionPoint{Row: 1, Col: codeOffset + 2}) {
		t.Fatalf("cursor after motion = %+v, want row 1 col %d", got, codeOffset+2)
	}

	cmd, err = viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventRelease,
		Row:       1,
		Col:       codeOffset + 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("release command = %v, want %v", cmd, CommandRedraw)
	}

	if got, want := viewer.ClipboardText(), "bcde\nfgh"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
}

func TestDiffViewerMouseSelectionCopiesOnlyCode(t *testing.T) {
	rows := []diff.Row{
		{
			Kind:   diff.RowAdd,
			Gutter: "1 1 + ",
			Code:   "hello",
		},
		{
			Kind:   diff.RowDelete,
			Gutter: "2 2 - ",
			Code:   "world",
		},
	}
	for i := range rows {
		rows[i].Text = rows[i].Gutter + rows[i].Code
	}
	viewer := &diffViewer{rows: rows}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))
	codeOffset := testCodeOffset(rows[0])

	cmd, err := viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventPress,
		Row:       0,
		Col:       0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("press command = %v, want %v", cmd, CommandRedraw)
	}

	cmd, err = viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventMotion,
		Row:       1,
		Col:       codeOffset + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("motion command = %v, want %v", cmd, CommandRedraw)
	}

	cmd, err = viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventRelease,
		Row:       1,
		Col:       codeOffset + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("release command = %v, want %v", cmd, CommandRedraw)
	}

	if got, want := viewer.ClipboardText(), "hello\nwo"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
	if viewer.mode != modeVisual {
		t.Fatalf("mode = %v, want visual", viewer.mode)
	}
}

func TestDiffViewerClickDoesNotSelect(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{Kind: diff.RowContext, Text: "abcde", Code: "abcde"}},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	cmd, err := viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventPress,
		Row:       0,
		Col:       2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("press command = %v, want %v", cmd, CommandRedraw)
	}
	if viewer.selection.Active {
		t.Fatalf("selection active after press: %+v", viewer.selection)
	}

	cmd, err = viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventRelease,
		Row:       0,
		Col:       2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandNone {
		t.Fatalf("release command = %v, want %v", cmd, CommandNone)
	}
	if viewer.selection.Active {
		t.Fatalf("selection active after click: %+v", viewer.selection)
	}
}

func TestDiffViewerCellDragStartsSelection(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{Kind: diff.RowContext, Text: "abcdef", Code: "abcdef"}},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	_, err := viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventPress,
		Row:       0,
		Col:       1,
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd, err := viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventMotion,
		Row:       0,
		Col:       1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandNone {
		t.Fatalf("small motion command = %v, want %v", cmd, CommandNone)
	}
	if viewer.selection.Active {
		t.Fatalf("selection active before cell drag: %+v", viewer.selection)
	}

	cmd, err = viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventMotion,
		Row:       0,
		Col:       2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("threshold motion command = %v, want %v", cmd, CommandRedraw)
	}
	if !viewer.selection.Active {
		t.Fatal("selection inactive after cell drag")
	}
	if got, want := viewer.ClipboardText(), "bc"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
}

func TestDiffViewerDragFromHunkHeaderIntoCodeStartsSelection(t *testing.T) {
	hunk := diff.Row{
		Kind:   diff.RowHunk,
		Text:   "@@ -1 +1 @@ func main()",
		Prefix: "@@ -1 +1 @@",
		Code:   " func main()",
	}
	code := diff.Row{Kind: diff.RowContext, Text: "abcdef", Code: "abcdef"}
	viewer := &diffViewer{rows: []diff.Row{hunk, code}}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	cmd, err := viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventPress,
		Row:       0,
		Col:       textCellWidth(hunk.Prefix) + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandNone {
		t.Fatalf("press command = %v, want %v", cmd, CommandNone)
	}
	if viewer.selection.Active {
		t.Fatalf("selection active after hunk press: %+v", viewer.selection)
	}

	cmd, err = viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventMotion,
		Row:       1,
		Col:       1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("motion command = %v, want %v", cmd, CommandRedraw)
	}
	if !viewer.selection.Active {
		t.Fatal("selection inactive after dragging into code")
	}
	if got, want := viewer.ClipboardText(), "ab"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}

	cmd, err = viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventMotion,
		Row:       1,
		Col:       2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("second motion command = %v, want %v", cmd, CommandRedraw)
	}
	if got, want := viewer.ClipboardText(), "abc"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
}

func TestDiffViewerDragFromHunkHeaderUpIntoCodeStartsAtEndOfLine(t *testing.T) {
	code := diff.Row{Kind: diff.RowContext, Text: "abcdef", Code: "abcdef"}
	hunk := diff.Row{
		Kind:   diff.RowHunk,
		Text:   "@@ -1 +1 @@ func main()",
		Prefix: "@@ -1 +1 @@",
		Code:   " func main()",
	}
	viewer := &diffViewer{rows: []diff.Row{code, hunk}}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	cmd, err := viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventPress,
		Row:       1,
		Col:       textCellWidth(hunk.Prefix) + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandNone {
		t.Fatalf("press command = %v, want %v", cmd, CommandNone)
	}

	cmd, err = viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventMotion,
		Row:       0,
		Col:       4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("motion command = %v, want %v", cmd, CommandRedraw)
	}
	if got, want := viewer.ClipboardText(), "ef"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}

	cmd, err = viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventMotion,
		Row:       0,
		Col:       3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("second motion command = %v, want %v", cmd, CommandRedraw)
	}
	if got, want := viewer.ClipboardText(), "def"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
}

func TestDiffViewerSelectionDoesNotSelectHunkHeaderContext(t *testing.T) {
	row := diff.Row{
		Kind:   diff.RowHunk,
		Text:   "@@ -231,8 +233,10 @@ func (d *diffViewer) Layout(constraints Constraints) Size {",
		Prefix: "@@ -231,8 +233,10 @@",
		Code:   " func (d *diffViewer) Layout(constraints Constraints) Size {",
	}
	viewer := &diffViewer{rows: []diff.Row{row}}
	viewer.Layout(Tight(Size{Width: 100, Height: 10}))

	if _, _, ok := viewer.codeRange(row); ok {
		t.Fatal("hunk header context is selectable")
	}
	if _, ok := viewer.selectionPoint(vaxis.Mouse{Row: 0, Col: textCellWidth(row.Prefix) + 1}); ok {
		t.Fatal("selection point found for hunk header context")
	}

	viewer.selection = textSelection{
		Active: true,
		Anchor: selectionPoint{Row: 0, Col: textCellWidth(row.Prefix)},
		Cursor: selectionPoint{Row: 0, Col: textCellWidth(row.Text) - 1},
	}
	if got := viewer.ClipboardText(); got != "" {
		t.Fatalf("clipboard text = %q, want empty", got)
	}
}

func TestDiffViewerClipboardSkipsNonSelectableRowsWithoutExtraNewline(t *testing.T) {
	rows := []diff.Row{
		{Kind: diff.RowContext, Code: "hello", Text: "hello"},
		{
			Kind:   diff.RowHunk,
			Text:   "@@ -1 +1 @@ func main()",
			Prefix: "@@ -1 +1 @@",
			Code:   " func main()",
		},
		{Kind: diff.RowContext, Code: "world", Text: "world"},
	}
	viewer := &diffViewer{
		rows: rows,
		selection: textSelection{
			Active: true,
			Anchor: selectionPoint{Row: 0, Col: 0},
			Cursor: selectionPoint{Row: 1, Col: textCellWidth(rows[1].Text) - 1},
		},
	}
	if got, want := viewer.ClipboardText(), "hello"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}

	viewer.selection.Cursor = selectionPoint{Row: 2, Col: textCellWidth(rows[2].Text) - 1}
	if got, want := viewer.ClipboardText(), "hello\nworld"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
}

func TestDiffViewerClipboardPreservesTabs(t *testing.T) {
	row := diff.Row{
		Kind: diff.RowContext,
		Code: "a\tb",
		Text: "a\tb",
	}
	viewer := &diffViewer{
		rows: []diff.Row{row},
		selection: textSelection{
			Active: true,
			Anchor: selectionPoint{Row: 0, Col: 1},
			Cursor: selectionPoint{Row: 0, Col: 1},
		},
	}

	if got, want := viewer.ClipboardText(), "\t"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
}

func TestDiffViewerTripleClickSelectsOnlyCode(t *testing.T) {
	row := diff.Row{
		Kind:   diff.RowAdd,
		Gutter: "1 1 + ",
		Code:   "hello",
	}
	row.Text = row.Gutter + row.Code
	viewer := &diffViewer{rows: []diff.Row{row}}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))
	codeOffset := testCodeOffset(row)

	for i := 0; i < 3; i++ {
		_, err := viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventPress,
			Row:       0,
			Col:       codeOffset + 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventRelease,
			Row:       0,
			Col:       codeOffset + 1,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if got, want := viewer.ClipboardText(), "hello"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
	if viewer.mode != modeVisualLine {
		t.Fatalf("mode = %v, want visual line", viewer.mode)
	}
}

func TestDiffViewerPaintsSelectedEmptyDiffLineAsOneCellAfterGutter(t *testing.T) {
	row := diff.Row{
		Kind:   diff.RowAdd,
		Gutter: "1 1 + ",
		Code:   "",
	}
	row.Text = row.Gutter
	gutterWidth := testCodeOffset(row)
	viewer := &diffViewer{
		rows: []diff.Row{row},
		selection: textSelection{
			Active: true,
			Anchor: selectionPoint{Row: 0, Col: 0},
			Cursor: selectionPoint{Row: 0, Col: gutterWidth},
		},
	}
	start, end, ok := viewer.selectionRange()
	if !ok {
		t.Fatal("selection range not active")
	}

	startCol, endCol, ok := viewer.selectionPaintRange(0, start, end)
	if !ok {
		t.Fatal("selection paint range not found")
	}
	if startCol != gutterWidth || endCol != gutterWidth+1 {
		t.Fatalf("paint range = %d:%d, want %d:%d", startCol, endCol, gutterWidth, gutterWidth+1)
	}
	if got, want := viewer.ClipboardText(), " "; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
}

func TestDiffViewerPaintsSelectedEmptyCodeAsOneCell(t *testing.T) {
	row := diff.Row{
		Kind:   diff.RowAdd,
		Gutter: "1 1 + ",
		Code:   "",
	}
	row.Text = row.Gutter
	codeOffset := testCodeOffset(row)
	viewer := &diffViewer{
		rows: []diff.Row{row},
		selection: textSelection{
			Active: true,
			Anchor: selectionPoint{Row: 0, Col: codeOffset},
			Cursor: selectionPoint{Row: 0, Col: codeOffset},
		},
	}
	start, end, ok := viewer.selectionRange()
	if !ok {
		t.Fatal("selection range not active")
	}

	startCol, endCol, ok := viewer.selectionPaintRange(0, start, end)
	if !ok {
		t.Fatal("selection paint range not found")
	}
	if startCol != codeOffset || endCol != codeOffset+1 {
		t.Fatalf("paint range = %d:%d, want %d:%d", startCol, endCol, codeOffset, codeOffset+1)
	}
}

func TestDiffViewerDoubleClickSelectsToken(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{Kind: diff.RowContext, Text: "foo bar.baz", Code: "foo bar.baz"}},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	for i := 0; i < 2; i++ {
		cmd, err := viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventPress,
			Row:       0,
			Col:       5,
		})
		if err != nil {
			t.Fatal(err)
		}
		if cmd != CommandRedraw {
			t.Fatalf("press %d command = %v, want %v", i, cmd, CommandRedraw)
		}
		_, err = viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventRelease,
			Row:       0,
			Col:       5,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if got, want := viewer.ClipboardText(), "bar"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
	if got, want := viewer.cursor, (selectionPoint{Row: 0, Col: 6}); got != want {
		t.Fatalf("cursor = %+v, want %+v", got, want)
	}
}

func TestDiffViewerTripleClickSelectsRow(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{Kind: diff.RowContext, Text: "foo bar.baz", Code: "foo bar.baz"}},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	for i := 0; i < 3; i++ {
		_, err := viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventPress,
			Row:       0,
			Col:       5,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventRelease,
			Row:       0,
			Col:       5,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if got, want := viewer.ClipboardText(), "foo bar.baz"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
	if got, want := viewer.cursor, (selectionPoint{Row: 0, Col: 5}); got != want {
		t.Fatalf("cursor = %+v, want %+v", got, want)
	}
}

func TestTokenRangeAtUsesUnicodeClasses(t *testing.T) {
	start, end := tokenRangeAt("alpha βeta.gamma", 7)
	if got, want := cellTextRange("alpha βeta.gamma", start, end), "βeta"; got != want {
		t.Fatalf("token = %q, want %q", got, want)
	}

	start, end = tokenRangeAt("alpha βeta.gamma", 10)
	if got, want := cellTextRange("alpha βeta.gamma", start, end), "."; got != want {
		t.Fatalf("punctuation token = %q, want %q", got, want)
	}
}

func TestDiffViewerCopyKeyCopiesSelection(t *testing.T) {
	tests := []struct {
		name string
		key  vaxis.Key
	}{
		{
			name: "y",
			key:  vaxis.Key{Text: "y", Keycode: 'y'},
		},
		{
			name: "copy key",
			key:  vaxis.Key{Keycode: vaxis.KeyCopy},
		},
		{
			name: "super c",
			key:  vaxis.Key{Text: "c", Keycode: 'c', Modifiers: vaxis.ModSuper},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viewer := &diffViewer{
				rows: []diff.Row{{Kind: diff.RowContext, Text: "abcde", Code: "abcde"}},
				selection: textSelection{
					Active: true,
					Anchor: selectionPoint{
						Row: 0,
						Col: 1,
					},
					Cursor: selectionPoint{
						Row: 0,
						Col: 3,
					},
				},
			}

			cmd, err := viewer.HandleEvent(tt.key)
			if err != nil {
				t.Fatal(err)
			}
			if cmd != CommandCopy {
				t.Fatalf("command = %v, want %v", cmd, CommandCopy)
			}
			if viewer.selection.Active {
				t.Fatalf("selection still active after copy: %+v", viewer.selection)
			}
			if viewer.mode != modeNormal {
				t.Fatalf("mode = %v, want normal", viewer.mode)
			}
			if got, want := viewer.ClipboardText(), "bcd"; got != want {
				t.Fatalf("clipboard text = %q, want %q", got, want)
			}
			viewer.ClipboardConsumed()
			if got := viewer.ClipboardText(); got != "" {
				t.Fatalf("clipboard text after consume = %q, want empty", got)
			}
		})
	}
}

func TestDiffViewerHighlightYankChangesSelectionStyleTemporarily(t *testing.T) {
	viewer := &diffViewer{}
	viewer.ensureColorScheme()
	now := time.Unix(1, 0)

	if got, want := viewer.selectionStyleAt(now).Background, viewer.scheme.Selection; got != want {
		t.Fatalf("selection background = %v, want %v", got, want)
	}

	viewer.HighlightYank(now)
	if got, want := viewer.selectionStyleAt(now.Add(time.Millisecond)).Background, viewer.scheme.Yank; got != want {
		t.Fatalf("yank background = %v, want %v", got, want)
	}
	if got, want := viewer.selectionStyleAt(now.Add(yankHighlightDuration+time.Millisecond)).Background, viewer.scheme.Selection; got != want {
		t.Fatalf("expired yank background = %v, want %v", got, want)
	}
}

func TestDiffViewerSelectionPointAccountsForHorizontalScroll(t *testing.T) {
	row := diff.Row{
		Kind:   diff.RowAdd,
		Gutter: "1 1 + ",
		Code:   "abcdef",
	}
	row.Text = row.Gutter + row.Code
	viewer := &diffViewer{
		rows:    []diff.Row{row},
		xScroll: 2,
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))
	codeOffset := testCodeOffset(row)

	point, ok := viewer.selectionPoint(vaxis.Mouse{
		Row: 0,
		Col: codeOffset,
	})
	if !ok {
		t.Fatal("selection point not found")
	}
	if want := codeOffset + viewer.xScroll; point.Col != want {
		t.Fatalf("selection col = %d, want %d", point.Col, want)
	}
}

func TestDiffViewerScrollbar(t *testing.T) {
	tests := []struct {
		name   string
		rows   int
		height int
		width  int
		scroll int
		want   scrollbar
	}{
		{
			name:   "hidden when rows fit",
			rows:   5,
			height: 10,
			width:  80,
			want:   scrollbar{},
		},
		{
			name:   "top",
			rows:   100,
			height: 11,
			width:  80,
			want: scrollbar{
				Visible: true,
				Col:     79,
				Row:     0,
				Length:  10,
				Thumb:   0,
				Size:    1,
			},
		},
		{
			name:   "middle",
			rows:   100,
			height: 11,
			width:  80,
			scroll: 45,
			want: scrollbar{
				Visible: true,
				Col:     79,
				Row:     0,
				Length:  10,
				Thumb:   4,
				Size:    1,
			},
		},
		{
			name:   "bottom",
			rows:   100,
			height: 11,
			width:  80,
			scroll: 90,
			want: scrollbar{
				Visible: true,
				Col:     79,
				Row:     0,
				Length:  10,
				Thumb:   9,
				Size:    1,
			},
		},
		{
			name:   "larger thumb",
			rows:   20,
			height: 11,
			width:  80,
			want: scrollbar{
				Visible: true,
				Col:     79,
				Row:     0,
				Length:  10,
				Thumb:   0,
				Size:    5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viewer := newTestDiffViewer(tt.rows, tt.height)
			viewer.scroll = tt.scroll
			got := viewer.scrollbar(tt.width, tt.height)

			if got != tt.want {
				t.Fatalf("scrollbar = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDiffViewerHorizontalScrollbar(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		content string
		xScroll int
		rows    int
		want    scrollbar
	}{
		{
			name:    "hidden when content fits",
			width:   20,
			height:  10,
			content: "short",
			want:    scrollbar{},
		},
		{
			name:    "top",
			width:   20,
			height:  10,
			content: "0123456789012345678901234567890123456789",
			want: scrollbar{
				Visible: true,
				Col:     0,
				Row:     8,
				Length:  20,
				Thumb:   0,
				Size:    10,
			},
		},
		{
			name:    "middle with vertical scrollbar",
			width:   20,
			height:  10,
			rows:    20,
			content: "0123456789012345678901234567890123456789",
			xScroll: 10,
			want: scrollbar{
				Visible: true,
				Col:     0,
				Row:     8,
				Length:  19,
				Thumb:   4,
				Size:    9,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := tt.rows
			if rows == 0 {
				rows = 1
			}
			viewer := newTestDiffViewer(rows, tt.height)
			viewer.width = tt.width
			viewer.rows[0].Text = tt.content
			viewer.xScroll = tt.xScroll
			got := viewer.horizontalScrollbar(tt.width, tt.height)

			if got != tt.want {
				t.Fatalf("horizontal scrollbar = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPaintSegmentsOffset(t *testing.T) {
	base := vaxis.Style{Foreground: vaxis.RGBColor(1, 2, 3)}
	alt := vaxis.Style{Foreground: vaxis.RGBColor(4, 5, 6)}
	segments := []vaxis.Segment{
		{Text: "abc", Style: base},
		{Text: "def", Style: alt},
		{Text: "ghi", Style: base},
	}
	cells := testCells{}

	paintSegmentsOffset(cells, 5, 0, 0, 2, segments...)

	if got, want := cells.text(5), "cdefg"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if got := cells[0].Style; got != base {
		t.Fatalf("first style = %+v, want %+v", got, base)
	}
	if got := cells[1].Style; got != alt {
		t.Fatalf("second style = %+v, want %+v", got, alt)
	}
	if got := cells[4].Style; got != base {
		t.Fatalf("last style = %+v, want %+v", got, base)
	}
}

func TestPaintSegmentsOffsetUsesCellWidths(t *testing.T) {
	base := vaxis.Style{Foreground: vaxis.RGBColor(1, 2, 3)}
	segments := []vaxis.Segment{{Text: "a\tb", Style: base}}
	cells := testCells{}

	paintSegmentsOffset(cells, 4, 0, 0, 1, segments...)

	if got, want := cells.text(4), "    "; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
}

func TestStyleSegmentsRangeAppliesCurlyUnderline(t *testing.T) {
	base := vaxis.Style{Foreground: vaxis.RGBColor(1, 2, 3)}
	underline := vaxis.Style{
		UnderlineColor: vaxis.RGBColor(4, 5, 6),
		UnderlineStyle: vaxis.UnderlineCurly,
	}

	segments := styleSegmentsRange([]vaxis.Segment{{Text: "abcdef", Style: base}}, 1, 4, underline)

	if got, want := segmentTextWidth(segments), 6; got != want {
		t.Fatalf("width = %d, want %d", got, want)
	}
	for index, segment := range segments {
		switch segment.Text {
		case "a", "ef":
			if segment.Style.UnderlineStyle != vaxis.UnderlineOff {
				t.Fatalf("segment %d = %+v, want no underline", index, segment)
			}
		case "bcd":
			if segment.Style.UnderlineStyle != vaxis.UnderlineCurly || segment.Style.UnderlineColor != underline.UnderlineColor {
				t.Fatalf("segment %d = %+v, want curly underline", index, segment)
			}
		default:
			t.Fatalf("unexpected segment %d = %+v", index, segment)
		}
	}
}

func TestDiffViewerReviewSegmentsUnderlineInlineDraft(t *testing.T) {
	start, end := 2, 5
	anchor := review.Anchor{Path: "main.go", Line: 12, Side: review.SideRight}
	viewer := &diffViewer{
		reviewDrafts: []review.CommentDraft{{
			Path:        "main.go",
			Line:        12,
			Side:        review.SideRight,
			StartColumn: &start,
			EndColumn:   &end,
			Body:        "inline",
		}},
	}
	viewer.ensureColorScheme()

	segments := viewer.reviewSegments(diff.Row{Review: anchor}, []vaxis.Segment{{Text: "abcdef", Style: viewer.baseStyle()}})

	if len(segments) != 3 {
		t.Fatalf("segments = %+v, want 3", segments)
	}
	if got, want := segments[1].Text, "bcde"; got != want {
		t.Fatalf("underlined text = %q, want %q", got, want)
	}
	if segments[1].Style.UnderlineStyle != vaxis.UnderlineCurly || segments[1].Style.UnderlineColor != viewer.scheme.Yellow {
		t.Fatalf("underlined style = %+v", segments[1].Style)
	}
}

func TestDiffViewerSelectionPreservesInlineReviewUnderline(t *testing.T) {
	start, end := 2, 5
	anchor := review.Anchor{Path: "main.go", Line: 12, Side: review.SideRight}
	row := diff.Row{
		Gutter: "1 1 + ",
		Code:   "abcdef",
		Review: anchor,
	}
	row.Text = row.Gutter + row.Code
	viewer := &diffViewer{
		rows: []diff.Row{row},
		reviewDrafts: []review.CommentDraft{{
			Path:        "main.go",
			Line:        12,
			Side:        review.SideRight,
			StartColumn: &start,
			EndColumn:   &end,
			Body:        "inline",
		}},
	}
	viewer.ensureColorScheme()

	style := viewer.selectionCellStyle(row, testCodeOffset(row)+1, viewer.selectionStyle())
	if style.UnderlineStyle != vaxis.UnderlineCurly || style.UnderlineColor != viewer.scheme.Yellow {
		t.Fatalf("selected inline style = %+v", style)
	}

	style = viewer.selectionCellStyle(row, testCodeOffset(row), viewer.selectionStyle())
	if style.UnderlineStyle != vaxis.UnderlineOff {
		t.Fatalf("selected non-inline style = %+v, want no underline", style)
	}
}

func TestSegmentTextWidthUsesCellWidths(t *testing.T) {
	got := segmentTextWidth([]vaxis.Segment{{Text: "a\tb"}})

	if got != 10 {
		t.Fatalf("width = %d, want 10", got)
	}
}

func newTestDiffViewer(rows int, height int) *diffViewer {
	viewer := &diffViewer{
		rows: make([]diff.Row, rows),
	}
	viewer.Layout(Tight(Size{
		Width:  80,
		Height: height,
	}))
	return viewer
}

func testCodeOffset(row diff.Row) int {
	return textCellWidth(row.Gutter + row.Marker)
}

func openReviewCommentEditor(t *testing.T, viewer *diffViewer) Command {
	t.Helper()
	cmd, err := viewer.HandleEvent(vaxis.Key{Text: "i", Keycode: 'i'})
	if err != nil {
		t.Fatal(err)
	}
	if viewer.editor == nil {
		t.Fatal("comment editor is nil after open")
	}
	if viewer.mode != modeInsert {
		t.Fatalf("mode = %v, want insert", viewer.mode)
	}
	return cmd
}

func submitReviewComment(t *testing.T, viewer *diffViewer, body string) Command {
	t.Helper()
	if body != "" {
		cmd, err := viewer.HandleEvent(vaxis.Key{Text: body})
		if err != nil {
			t.Fatal(err)
		}
		if cmd != CommandRedraw {
			t.Fatalf("text command = %v, want %v", cmd, CommandRedraw)
		}
	}
	cmd, err := viewer.HandleEvent(vaxis.Key{Keycode: vaxis.KeyEsc})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("escape command = %v, want %v", cmd, CommandRedraw)
	}
	return cmd
}

func executeCommand(t *testing.T, viewer *diffViewer, command string) Command {
	t.Helper()
	cmd, err := viewer.HandleEvent(vaxis.Key{Text: ":", Keycode: ':'})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("colon command = %v, want %v", cmd, CommandRedraw)
	}
	if viewer.mode != modeCommand {
		t.Fatalf("mode = %v, want command", viewer.mode)
	}
	if command != "" {
		cmd, err = viewer.HandleEvent(vaxis.Key{Text: command})
		if err != nil {
			t.Fatal(err)
		}
		if cmd != CommandRedraw {
			t.Fatalf("command text result = %v, want %v", cmd, CommandRedraw)
		}
	}
	cmd, err = viewer.HandleEvent(vaxis.Key{Keycode: vaxis.KeyEnter})
	if err != nil {
		t.Fatal(err)
	}
	return cmd
}

type testCells map[int]vaxis.Cell

func (t testCells) SetCell(col int, row int, cell vaxis.Cell) {
	t[col] = cell
}

func (t testCells) text(width int) string {
	var b strings.Builder
	for col := 0; col < width; col++ {
		b.WriteString(t[col].Character.Grapheme)
	}
	return b.String()
}

func TestApplyInlineSpans(t *testing.T) {
	base := vaxis.Style{
		Foreground: vaxis.RGBColor(1, 2, 3),
		Background: vaxis.RGBColor(4, 5, 6),
	}
	inlineBackground := vaxis.RGBColor(7, 8, 9)

	segments := applyInlineSpans([]vaxis.Segment{
		{Text: "foo ", Style: base},
		{Text: "bar", Style: base},
	}, []diff.InlineSpan{
		{Start: 4, End: 7, Kind: diff.InlineChange},
	}, inlineBackground)

	if len(segments) != 2 {
		t.Fatalf("segments = %+v, want 2 segments", segments)
	}
	if segments[0].Text != "foo " || segments[0].Style.Background != base.Background {
		t.Fatalf("first segment = %+v", segments[0])
	}
	if segments[1].Text != "bar" || segments[1].Style.Background != inlineBackground {
		t.Fatalf("second segment = %+v", segments[1])
	}
}
