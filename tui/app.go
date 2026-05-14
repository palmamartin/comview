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
		rows: doc.Rows(),
	}, vaxis.Options{})
	if err != nil {
		return err
	}

	return app.Run()
}

type diffViewer struct {
	rows   []diff.Row
	scroll int
	height int
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

func (d *diffViewer) Layout(size Size) Size {
	d.height = size.Height
	d.clampScroll()
	return size
}

func (d *diffViewer) Paint(win vaxis.Window) {
	width, height := win.Size()
	if width == 0 || height == 0 {
		return
	}

	headerStyle := vaxis.Style{
		Foreground: vaxis.IndexColor(14),
		Attribute:  vaxis.AttrBold,
	}
	mutedStyle := vaxis.Style{
		Foreground: vaxis.IndexColor(8),
	}

	printAt(win, 0, 0, "comview", headerStyle)

	if len(d.rows) == 0 {
		printAt(win, 0, 2, "Pipe git diff or git show into comview.", vaxis.Style{})
		printAt(win, 0, 4, "Press q, Esc, Ctrl+C, or Ctrl+D to quit.", mutedStyle)
		return
	}

	printAt(win, 10, 0, "j/k or arrows scroll, q quits", mutedStyle)

	for row, diffRow := range d.visibleRows() {
		printAt(win, 0, row+1, diffRow.Text, styleFor(diffRow.Kind))
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

func styleFor(kind diff.RowKind) vaxis.Style {
	switch kind {
	case diff.RowFile:
		return vaxis.Style{
			Foreground: vaxis.IndexColor(14),
			Attribute:  vaxis.AttrBold,
		}
	case diff.RowHunk:
		return vaxis.Style{
			Foreground: vaxis.IndexColor(5),
		}
	case diff.RowAdd:
		return vaxis.Style{
			Foreground: vaxis.IndexColor(2),
		}
	case diff.RowDelete:
		return vaxis.Style{
			Foreground: vaxis.IndexColor(1),
		}
	case diff.RowMeta, diff.RowPreamble, diff.RowNoNewline:
		return vaxis.Style{
			Foreground: vaxis.IndexColor(8),
		}
	default:
		return vaxis.Style{}
	}
}

func printAt(win vaxis.Window, col int, row int, text string, style vaxis.Style) {
	width, height := win.Size()
	if col >= width || row >= height {
		return
	}

	line := win.New(col, row, -1, 1)
	line.PrintTruncate(0, vaxis.Segment{
		Text:  text,
		Style: style,
	})
}
