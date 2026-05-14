package tui

import (
	"strings"
	"testing"
	"time"

	"git.sr.ht/~rockorager/vaxis"

	"github.com/rockorager/comview/diff"
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

func TestDiffViewerVimNavigationKeys(t *testing.T) {
	tests := []struct {
		name     string
		start    int
		key      vaxis.Key
		want     int
		wantCmd  Command
		pending  string
		wantPend string
	}{
		{
			name:    "G scrolls to bottom",
			key:     vaxis.Key{Text: "G", Keycode: 'G'},
			want:    91,
			wantCmd: CommandRedraw,
		},
		{
			name:    "End scrolls to bottom",
			key:     vaxis.Key{Keycode: vaxis.KeyEnd},
			want:    91,
			wantCmd: CommandRedraw,
		},
		{
			name:    "Home scrolls to top",
			start:   40,
			key:     vaxis.Key{Keycode: vaxis.KeyHome},
			want:    0,
			wantCmd: CommandRedraw,
		},
		{
			name:    "Ctrl+d scrolls down half page",
			start:   10,
			key:     vaxis.Key{Keycode: 'd', Modifiers: vaxis.ModCtrl},
			want:    14,
			wantCmd: CommandRedraw,
		},
		{
			name:    "Page Down scrolls down half page",
			start:   10,
			key:     vaxis.Key{Keycode: vaxis.KeyPgDown},
			want:    14,
			wantCmd: CommandRedraw,
		},
		{
			name:    "Ctrl+u scrolls up half page",
			start:   10,
			key:     vaxis.Key{Keycode: 'u', Modifiers: vaxis.ModCtrl},
			want:    6,
			wantCmd: CommandRedraw,
		},
		{
			name:    "Page Up scrolls up half page",
			start:   10,
			key:     vaxis.Key{Keycode: vaxis.KeyPgUp},
			want:    6,
			wantCmd: CommandRedraw,
		},
		{
			name:     "g waits for second g",
			start:    10,
			key:      vaxis.Key{Text: "g", Keycode: 'g'},
			want:     10,
			wantCmd:  CommandNone,
			wantPend: "g",
		},
		{
			name:    "second g scrolls to top",
			start:   10,
			key:     vaxis.Key{Text: "g", Keycode: 'g'},
			want:    0,
			wantCmd: CommandRedraw,
			pending: "g",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viewer := newTestDiffViewer(100, 10)
			viewer.scroll = tt.start
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
			if viewer.scroll != tt.want {
				t.Fatalf("scroll = %d, want %d", viewer.scroll, tt.want)
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
		name  string
		start int
		key   vaxis.Key
		want  int
	}{
		{
			name: "l scrolls right",
			key:  vaxis.Key{Text: "l", Keycode: 'l'},
			want: 1,
		},
		{
			name: "Right scrolls right",
			key:  vaxis.Key{Keycode: vaxis.KeyRight},
			want: 1,
		},
		{
			name:  "h scrolls left",
			start: 5,
			key:   vaxis.Key{Text: "h", Keycode: 'h'},
			want:  4,
		},
		{
			name:  "Left scrolls left",
			start: 5,
			key:   vaxis.Key{Keycode: vaxis.KeyLeft},
			want:  4,
		},
		{
			name:  "left clamps at zero",
			start: 0,
			key:   vaxis.Key{Keycode: vaxis.KeyLeft},
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viewer := newTestDiffViewer(3, 10)
			viewer.rows[0].Text = strings.Repeat("x", 120)
			viewer.xScroll = tt.start

			cmd, err := viewer.HandleEvent(tt.key)
			if err != nil {
				t.Fatal(err)
			}
			if cmd != CommandRedraw {
				t.Fatalf("command = %v, want %v", cmd, CommandRedraw)
			}
			if viewer.xScroll != tt.want {
				t.Fatalf("xScroll = %d, want %d", viewer.xScroll, tt.want)
			}
		})
	}
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
			mouse:  vaxis.Mouse{Button: vaxis.MouseLeftButton},
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
			if tt.mouse.Button == vaxis.MouseLeftButton && cmd != CommandNone {
				t.Fatalf("command = %v, want %v", cmd, CommandNone)
			}
			if tt.mouse.Button != vaxis.MouseLeftButton && cmd != CommandRedraw {
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
	if got := viewer.selection.Cursor; got != (selectionPoint{Row: 15, Col: 2}) {
		t.Fatalf("cursor = %+v, want row 15 col 2", got)
	}
}

func TestDiffViewerMouseWheelDoesNotExtendFinishedSelection(t *testing.T) {
	viewer := newTestDiffViewer(100, 10)
	for i := range viewer.rows {
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
	viewer := &diffViewer{
		rows: []diff.Row{
			{Kind: diff.RowContext, Text: "abcde"},
			{Kind: diff.RowContext, Text: "fghij"},
		},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	cmd, err := viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventPress,
		Row:       1,
		Col:       1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != CommandRedraw {
		t.Fatalf("press command = %v, want %v", cmd, CommandRedraw)
	}

	cmd, err = viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventRelease,
		Row:       2,
		Col:       2,
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

func TestDiffViewerAltMouseSelectionCopiesOnlyCode(t *testing.T) {
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
	codeOffset := textCellWidth(rows[0].Gutter + rows[0].Marker)

	cmd, err := viewer.HandleEvent(vaxis.Mouse{
		Button:    vaxis.MouseLeftButton,
		EventType: vaxis.EventPress,
		Modifiers: vaxis.ModAlt,
		Row:       1,
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
		EventType: vaxis.EventRelease,
		Row:       2,
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
}

func TestDiffViewerAltTripleClickSelectsOnlyCode(t *testing.T) {
	row := diff.Row{
		Kind:   diff.RowAdd,
		Gutter: "1 1 + ",
		Code:   "hello",
	}
	row.Text = row.Gutter + row.Code
	viewer := &diffViewer{rows: []diff.Row{row}}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))
	codeOffset := textCellWidth(row.Gutter + row.Marker)

	for i := 0; i < 3; i++ {
		_, err := viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventPress,
			Modifiers: vaxis.ModAlt,
			Row:       1,
			Col:       codeOffset + 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventRelease,
			Row:       1,
			Col:       codeOffset + 1,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if got, want := viewer.ClipboardText(), "hello"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
}

func TestDiffViewerPaintsSelectedEmptyLineAsOneCell(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{Kind: diff.RowContext, Text: ""}},
		selection: textSelection{
			Active: true,
			Anchor: selectionPoint{Row: 0, Col: 0},
			Cursor: selectionPoint{Row: 0, Col: 0},
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
	if startCol != 0 || endCol != 1 {
		t.Fatalf("paint range = %d:%d, want 0:1", startCol, endCol)
	}
}

func TestDiffViewerPaintsSelectedEmptyCodeAsOneCell(t *testing.T) {
	row := diff.Row{
		Kind:   diff.RowAdd,
		Gutter: "1 1 + ",
		Code:   "",
	}
	row.Text = row.Gutter
	codeOffset := textCellWidth(row.Gutter + row.Marker)
	viewer := &diffViewer{
		rows: []diff.Row{row},
		selection: textSelection{
			Active: true,
			Mode:   selectionCode,
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
		rows: []diff.Row{{Kind: diff.RowContext, Text: "foo bar.baz"}},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	for i := 0; i < 2; i++ {
		cmd, err := viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventPress,
			Row:       1,
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
			Row:       1,
			Col:       5,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if got, want := viewer.ClipboardText(), "bar"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
	}
}

func TestDiffViewerTripleClickSelectsRow(t *testing.T) {
	viewer := &diffViewer{
		rows: []diff.Row{{Kind: diff.RowContext, Text: "foo bar.baz"}},
	}
	viewer.Layout(Tight(Size{Width: 80, Height: 10}))

	for i := 0; i < 3; i++ {
		_, err := viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventPress,
			Row:       1,
			Col:       5,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = viewer.HandleEvent(vaxis.Mouse{
			Button:    vaxis.MouseLeftButton,
			EventType: vaxis.EventRelease,
			Row:       1,
			Col:       5,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if got, want := viewer.ClipboardText(), "foo bar.baz"; got != want {
		t.Fatalf("clipboard text = %q, want %q", got, want)
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
				rows: []diff.Row{{Kind: diff.RowContext, Text: "abcde"}},
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
	codeOffset := textCellWidth(row.Gutter + row.Marker)

	point, ok := viewer.selectionPoint(vaxis.Mouse{
		Row: 1,
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
				Row:     1,
				Length:  10,
				Thumb:   1,
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
				Row:     1,
				Length:  10,
				Thumb:   5,
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
				Row:     1,
				Length:  10,
				Thumb:   10,
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
				Row:     1,
				Length:  10,
				Thumb:   1,
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
				Row:     9,
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
				Row:     9,
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
