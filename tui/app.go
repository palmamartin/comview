package tui

import (
	"git.sr.ht/~rockorager/vaxis"

	"github.com/rockorager/comview/diff"
)

// Run starts the comview TUI.
func Run(input string) error {
	doc, err := diff.Parse(input)
	if err != nil {
		return err
	}

	app, err := NewApp(&diffViewer{
		rows:        doc.RowsWithOptions(diff.DefaultRenderOptions()),
		highlighter: NewSyntaxHighlighter(),
	}, vaxis.Options{})
	if err != nil {
		return err
	}

	return app.Run()
}

type diffViewer struct {
	rows        []diff.Row
	scroll      int
	height      int
	scheme      ColorScheme
	highlighter *SyntaxHighlighter
}

func (d *diffViewer) SetTerminalColors(colors TerminalColors) {
	d.ensureColorScheme()
	d.scheme.ApplyTerminalColors(colors)
	if d.highlighter != nil {
		d.highlighter.SetColorScheme(d.scheme)
	}
}

func (d *diffViewer) HandleEvent(ev vaxis.Event) (Command, error) {
	key, ok := ev.(vaxis.Key)
	if !ok {
		return CommandNone, nil
	}

	switch {
	case key.Matches('c', vaxis.ModCtrl),
		key.Matches('d', vaxis.ModCtrl),
		key.Matches('q'),
		key.MatchString("Esc"):
		return CommandQuit, nil
	case key.Matches('j'), key.MatchString("Down"):
		d.scroll++
		d.clampScroll()
		return CommandRedraw, nil
	case key.Matches('k'), key.MatchString("Up"):
		if d.scroll > 0 {
			d.scroll--
		}
		return CommandRedraw, nil
	default:
		return CommandNone, nil
	}
}

func (d *diffViewer) Layout(constraints Constraints) Size {
	size := constraints.Constrain(constraints.Max)
	d.height = size.Height
	d.clampScroll()
	return size
}

func (d *diffViewer) Paint(win vaxis.Window) {
	width, height := win.Size()
	if width == 0 || height == 0 {
		return
	}
	d.ensureColorScheme()

	headerStyle := vaxis.Style{
		Foreground: d.scheme.Header,
		Background: d.scheme.Background,
		Attribute:  vaxis.AttrBold,
	}
	mutedStyle := vaxis.Style{
		Foreground: d.scheme.Muted,
		Background: d.scheme.Background,
	}

	win.Fill(vaxis.Cell{
		Character: vaxis.Character{
			Grapheme: " ",
			Width:    1,
		},
		Style: vaxis.Style{
			Foreground: d.scheme.Foreground,
			Background: d.scheme.Background,
		},
	})
	printAt(win, 0, 0, "comview", headerStyle)

	if len(d.rows) == 0 {
		printAt(win, 0, 2, "Pipe git diff or git show into comview.", d.baseStyle())
		printAt(win, 0, 4, "Press q, Esc, Ctrl+C, or Ctrl+D to quit.", mutedStyle)
		return
	}

	printAt(win, 10, 0, "j/k or arrows scroll, q quits", mutedStyle)

	for row, diffRow := range d.visibleRows() {
		d.printRow(win, row+1, diffRow)
	}
}

func (d *diffViewer) printRow(win vaxis.Window, row int, diffRow diff.Row) {
	d.fillRowBackground(win, row, diffRow.Kind)
	if diffRow.Gutter != "" || diffRow.Marker != "" {
		segments := []vaxis.Segment{
			{Text: diffRow.Gutter, Style: d.gutterStyle(diffRow.Kind)},
			{Text: diffRow.Marker, Style: d.styleFor(diffRow.Kind)},
		}
		if diffRow.Code != "" {
			segments = append(segments, d.highlighter.Highlight(diffRow.FileName, diffRow.Code, d.codeStyle(diffRow.Kind))...)
		}
		printSegmentsAt(win, 0, row, segments...)
		return
	}

	if diffRow.Code == "" {
		printAt(win, 0, row, diffRow.Text, d.styleFor(diffRow.Kind))
		return
	}

	style := d.styleFor(diffRow.Kind)
	segments := []vaxis.Segment{
		{Text: diffRow.Gutter, Style: d.styleFor(diff.RowMeta)},
		{Text: diffRow.Marker, Style: style},
	}
	segments = append(segments, d.highlighter.Highlight(diffRow.FileName, diffRow.Code, d.codeStyle(diffRow.Kind))...)
	printSegmentsAt(win, 0, row, segments...)
}

func (d *diffViewer) fillRowBackground(win vaxis.Window, row int, kind diff.RowKind) {
	if kind != diff.RowAdd && kind != diff.RowDelete {
		return
	}

	style := d.styleFor(kind)
	width, height := win.Size()
	if row >= height {
		return
	}

	for col := 0; col < width; col++ {
		win.SetCell(col, row, vaxis.Cell{
			Character: vaxis.Character{
				Grapheme: " ",
				Width:    1,
			},
			Style: style,
		})
	}
}

func (d *diffViewer) visibleRows() []diff.Row {
	if d.height <= 1 || d.scroll >= len(d.rows) {
		return nil
	}

	available := d.height - 1
	end := d.scroll + available
	if end > len(d.rows) {
		end = len(d.rows)
	}
	return d.rows[d.scroll:end]
}

func (d *diffViewer) clampScroll() {
	maxScroll := len(d.rows) - 1
	if visible := d.height - 1; visible > 0 {
		maxScroll = len(d.rows) - visible
	}
	if maxScroll < 0 {
		maxScroll = 0
	}
	if d.scroll > maxScroll {
		d.scroll = maxScroll
	}
}

func (d *diffViewer) styleFor(kind diff.RowKind) vaxis.Style {
	switch kind {
	case diff.RowFile:
		return vaxis.Style{
			Foreground: d.scheme.Header,
			Background: d.scheme.Background,
			Attribute:  vaxis.AttrBold,
		}
	case diff.RowHunk:
		return vaxis.Style{
			Foreground: d.scheme.Hunk,
			Background: d.scheme.Background,
		}
	case diff.RowAdd:
		return vaxis.Style{
			Foreground: d.scheme.Add,
			Background: d.scheme.AddLine,
		}
	case diff.RowDelete:
		return vaxis.Style{
			Foreground: d.scheme.Delete,
			Background: d.scheme.DeleteLine,
		}
	case diff.RowMeta, diff.RowPreamble, diff.RowNoNewline:
		return vaxis.Style{
			Foreground: d.scheme.Muted,
			Background: d.scheme.Background,
		}
	default:
		return d.baseStyle()
	}
}

func (d *diffViewer) ensureColorScheme() {
	if d.scheme.Foreground == vaxis.ColorDefault {
		d.scheme = DefaultColorScheme()
	}
}

func (d *diffViewer) baseStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Foreground,
		Background: d.scheme.Background,
	}
}

func (d *diffViewer) codeStyle(kind diff.RowKind) vaxis.Style {
	style := d.baseStyle()
	switch kind {
	case diff.RowAdd:
		style.Background = d.scheme.AddLine
	case diff.RowDelete:
		style.Background = d.scheme.DeleteLine
	}
	return style
}

func (d *diffViewer) gutterStyle(kind diff.RowKind) vaxis.Style {
	style := vaxis.Style{
		Foreground: d.scheme.Muted,
		Background: d.scheme.Background,
	}
	switch kind {
	case diff.RowAdd:
		style.Background = d.scheme.AddLine
	case diff.RowDelete:
		style.Background = d.scheme.DeleteLine
	}
	return style
}

func printAt(win vaxis.Window, col int, row int, text string, style vaxis.Style) {
	printSegmentsAt(win, col, row, vaxis.Segment{
		Text:  text,
		Style: style,
	})
}

func printSegmentsAt(win vaxis.Window, col int, row int, segments ...vaxis.Segment) {
	width, height := win.Size()
	if col >= width || row >= height {
		return
	}

	line := win.New(col, row, -1, 1)
	line.PrintTruncate(0, segments...)
}
