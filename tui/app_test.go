package tui

import (
	"testing"

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
		scheme.Dim,
		scheme.Header,
		scheme.Muted,
		scheme.Hunk,
		scheme.Blue,
		scheme.Yellow,
		scheme.Add,
		scheme.AddLine,
		scheme.AddInline,
		scheme.Delete,
		scheme.DeleteLine,
		scheme.DeleteInline,
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

func TestDiffViewerUsesMutedGutterForegroundForChangedLines(t *testing.T) {
	viewer := &diffViewer{}
	viewer.ensureColorScheme()

	addGutter := viewer.gutterStyle(diff.RowAdd)
	if addGutter.Foreground != viewer.scheme.Muted {
		t.Fatalf("add gutter foreground = %v, want %v", addGutter.Foreground, viewer.scheme.Muted)
	}
	if addGutter.Background != viewer.scheme.AddLine {
		t.Fatalf("add gutter background = %v, want %v", addGutter.Background, viewer.scheme.AddLine)
	}

	deleteGutter := viewer.gutterStyle(diff.RowDelete)
	if deleteGutter.Foreground != viewer.scheme.Muted {
		t.Fatalf("delete gutter foreground = %v, want %v", deleteGutter.Foreground, viewer.scheme.Muted)
	}
	if deleteGutter.Background != viewer.scheme.DeleteLine {
		t.Fatalf("delete gutter background = %v, want %v", deleteGutter.Background, viewer.scheme.DeleteLine)
	}
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
