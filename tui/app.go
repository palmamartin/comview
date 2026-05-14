package tui

import (
	"time"

	"git.sr.ht/~rockorager/vaxis"

	"github.com/rockorager/comview/diff"
)

const pendingKeyTimeout = 800 * time.Millisecond
const mouseWheelScrollLines = 1
const scrollbarWidth = 1
const verticalScrollbarThumb = "█"
const horizontalScrollbarThumb = "\U0001FB0B"

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
	rows         []diff.Row
	scroll       int
	xScroll      int
	height       int
	width        int
	contentWide  int
	codeSegments [][]vaxis.Segment
	keys         keyChordState
	scheme       ColorScheme
	highlighter  *SyntaxHighlighter
}

func (d *diffViewer) SetTerminalColors(colors TerminalColors) {
	d.ensureColorScheme()
	d.scheme.ApplyTerminalColors(colors)
	if d.highlighter != nil {
		d.highlighter.SetColorScheme(d.scheme)
	}
	d.invalidateRenderCache()
}

func (d *diffViewer) HandleEvent(ev vaxis.Event) (Command, error) {
	switch ev := ev.(type) {
	case vaxis.Key:
		return d.handleKey(ev)
	case vaxis.Mouse:
		return d.handleMouse(ev)
	default:
		return CommandNone, nil
	}
}

func (d *diffViewer) handleKey(key vaxis.Key) (Command, error) {
	d.keys.ClearExpired(time.Now())

	switch {
	case key.Matches('c', vaxis.ModCtrl),
		key.Matches('q'),
		key.MatchString("Esc"):
		return CommandQuit, nil
	case key.Matches('g'):
		if d.keys.Pending() == "g" {
			d.keys.Clear()
			d.scrollTop()
			return CommandRedraw, nil
		}
		d.keys.Set("g", time.Now())
		return CommandNone, nil
	case key.Matches('G'), key.Matches(vaxis.KeyEnd):
		d.keys.Clear()
		d.scrollBottom()
		return CommandRedraw, nil
	case key.Matches(vaxis.KeyHome):
		d.keys.Clear()
		d.scrollTop()
		return CommandRedraw, nil
	case key.Matches('d', vaxis.ModCtrl), key.Matches(vaxis.KeyPgDown):
		d.keys.Clear()
		d.scrollBy(d.halfPage())
		return CommandRedraw, nil
	case key.Matches('u', vaxis.ModCtrl), key.Matches(vaxis.KeyPgUp):
		d.keys.Clear()
		d.scrollBy(-d.halfPage())
		return CommandRedraw, nil
	case key.Matches('j'), key.MatchString("Down"):
		d.keys.Clear()
		d.scrollBy(1)
		return CommandRedraw, nil
	case key.Matches('k'), key.MatchString("Up"):
		d.keys.Clear()
		d.scrollBy(-1)
		return CommandRedraw, nil
	case key.Matches('l'), key.Matches(vaxis.KeyRight):
		d.keys.Clear()
		d.scrollHorizontalBy(1)
		return CommandRedraw, nil
	case key.Matches('h'), key.Matches(vaxis.KeyLeft):
		d.keys.Clear()
		d.scrollHorizontalBy(-1)
		return CommandRedraw, nil
	default:
		d.keys.Clear()
		return CommandNone, nil
	}
}

func (d *diffViewer) handleMouse(mouse vaxis.Mouse) (Command, error) {
	switch mouse.Button {
	case vaxis.MouseWheelDown:
		d.keys.Clear()
		d.scrollBy(mouseWheelScrollLines)
		return CommandRedraw, nil
	case vaxis.MouseWheelUp:
		d.keys.Clear()
		d.scrollBy(-mouseWheelScrollLines)
		return CommandRedraw, nil
	default:
		return CommandNone, nil
	}
}

func (d *diffViewer) Layout(constraints Constraints) Size {
	size := constraints.Constrain(constraints.Max)
	d.height = size.Height
	d.width = size.Width
	d.clampScroll()
	d.clampHorizontalScroll()
	return size
}

func (d *diffViewer) Paint(win vaxis.Window) {
	width, height := win.Size()
	if width == 0 || height == 0 {
		return
	}
	d.ensureColorScheme()
	d.ensureRenderCache()

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

	printAt(win, 10, 0, "j/k/h/l, gg/G, Ctrl+d/u scroll, q quits", mutedStyle)

	for row, diffRow := range d.visibleRows() {
		d.printRow(win, row+1, diffRow, d.codeSegments[d.scroll+row])
	}
	d.paintScrollbar(win)
	d.paintHorizontalScrollbar(win)
}

func (d *diffViewer) printRow(win vaxis.Window, row int, diffRow diff.Row, codeSegments []vaxis.Segment) {
	d.fillRowBackground(win, row, diffRow.Kind)
	if diffRow.Kind == diff.RowHunk && diffRow.Prefix != "" && diffRow.Code != "" {
		segments := []vaxis.Segment{
			{Text: diffRow.Prefix, Style: d.styleFor(diff.RowHunk)},
			{Text: diffRow.Code, Style: d.dimStyle()},
		}
		printSegmentsAt(win, 0, row, segments...)
		return
	}

	if diffRow.Gutter != "" || diffRow.Marker != "" {
		segments := []vaxis.Segment{
			{Text: diffRow.Gutter, Style: d.gutterStyle(diffRow.Kind)},
			{Text: diffRow.Marker, Style: d.styleFor(diffRow.Kind)},
		}
		if diffRow.Code != "" {
			codeOffset := segmentTextWidth(segments)
			printSegmentsAt(win, 0, row, segments...)
			printCodeSegmentsAtOffset(win, codeOffset, row, d.xScroll, codeSegments...)
			return
		}
		printSegmentsAt(win, 0, row, segments...)
		return
	}

	if diffRow.Code == "" {
		printSegmentsAt(win, 0, row, vaxis.Segment{Text: diffRow.Text, Style: d.styleFor(diffRow.Kind)})
		return
	}

	style := d.styleFor(diffRow.Kind)
	segments := []vaxis.Segment{
		{Text: diffRow.Gutter, Style: d.styleFor(diff.RowMeta)},
		{Text: diffRow.Marker, Style: style},
	}
	printSegmentsAt(win, 0, row, segments...)
	printCodeSegmentsAtOffset(win, 0, row, d.xScroll, codeSegments...)
}

func (d *diffViewer) ensureRenderCache() {
	if len(d.codeSegments) == len(d.rows) {
		return
	}

	highlightedRows := d.highlighter.HighlightRows(d.rows, d.codeStyle)
	d.codeSegments = make([][]vaxis.Segment, len(d.rows))
	for index, row := range d.rows {
		if row.Code == "" || row.Kind == diff.RowHunk {
			continue
		}

		segments := highlightedRows[index]
		if len(segments) == 0 {
			segments = d.highlighter.Highlight(row.FileName, row.Code, d.codeStyle(row.Kind))
		}
		d.codeSegments[index] = applyInlineSpans(segments, row.InlineSpans, d.inlineBackground(row.Kind))
	}
}

func (d *diffViewer) invalidateRenderCache() {
	d.codeSegments = nil
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

	available := d.visibleRowCapacity()
	end := d.scroll + available
	if end > len(d.rows) {
		end = len(d.rows)
	}
	return d.rows[d.scroll:end]
}

type scrollbar struct {
	Visible bool
	Col     int
	Row     int
	Length  int
	Thumb   int
	Size    int
}

func (d *diffViewer) scrollbar(width int, height int) scrollbar {
	trackTop := 1
	verticalVisible, _ := d.scrollbarVisibility(width, height)
	if !verticalVisible {
		return scrollbar{}
	}
	trackHeight := d.verticalTrackHeight(width, height)
	visibleRows := trackHeight
	totalRows := len(d.rows)
	if width <= 0 || trackHeight <= 0 || totalRows <= visibleRows {
		return scrollbar{}
	}

	thumbSize := (visibleRows * trackHeight) / totalRows
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > trackHeight {
		thumbSize = trackHeight
	}

	maxThumbTop := trackHeight - thumbSize
	thumbTop := 0
	if maxScroll := d.maxScroll(); maxScroll > 0 {
		thumbTop = (d.scroll * maxThumbTop) / maxScroll
	}

	return scrollbar{
		Visible: true,
		Col:     width - scrollbarWidth,
		Row:     trackTop,
		Length:  trackHeight,
		Thumb:   trackTop + thumbTop,
		Size:    thumbSize,
	}
}

func (d *diffViewer) verticalTrackHeight(width int, height int) int {
	trackHeight := height - 1
	_, horizontalVisible := d.scrollbarVisibility(width, height)
	if horizontalVisible {
		trackHeight--
	}
	return trackHeight
}

func (d *diffViewer) paintScrollbar(win vaxis.Window) {
	width, height := win.Size()
	bar := d.scrollbar(width, height)
	if !bar.Visible {
		return
	}

	trackStyle := vaxis.Style{
		Foreground: d.scheme.Dim,
		Background: d.scheme.Background,
	}
	thumbStyle := vaxis.Style{
		Foreground: d.scheme.Muted,
		Background: d.scheme.Background,
	}
	for row := bar.Row; row < bar.Row+bar.Length; row++ {
		style := trackStyle
		grapheme := "│"
		if row >= bar.Thumb && row < bar.Thumb+bar.Size {
			style = thumbStyle
			grapheme = verticalScrollbarThumb
		}
		win.SetCell(bar.Col, row, vaxis.Cell{
			Character: vaxis.Character{
				Grapheme: grapheme,
				Width:    scrollbarWidth,
			},
			Style: style,
		})
	}
}

func (d *diffViewer) horizontalScrollbar(width int, height int) scrollbar {
	verticalVisible, horizontalVisible := d.scrollbarVisibility(width, height)
	trackWidth := horizontalViewportWidth(width, verticalVisible)
	if width <= 0 || height <= 1 || trackWidth <= 0 {
		return scrollbar{}
	}

	contentWidth := d.contentWidth()
	if !horizontalVisible || contentWidth <= trackWidth {
		return scrollbar{}
	}

	thumbSize := (trackWidth * trackWidth) / contentWidth
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > trackWidth {
		thumbSize = trackWidth
	}

	maxThumbLeft := trackWidth - thumbSize
	thumbLeft := 0
	if maxScroll := d.maxHorizontalScroll(); maxScroll > 0 {
		thumbLeft = (d.xScroll * maxThumbLeft) / maxScroll
	}

	return scrollbar{
		Visible: true,
		Col:     0,
		Row:     height - 1,
		Length:  trackWidth,
		Thumb:   thumbLeft,
		Size:    thumbSize,
	}
}

func (d *diffViewer) paintHorizontalScrollbar(win vaxis.Window) {
	width, height := win.Size()
	bar := d.horizontalScrollbar(width, height)
	if !bar.Visible {
		return
	}

	trackStyle := vaxis.Style{
		Foreground: d.scheme.Dim,
		Background: d.scheme.Background,
	}
	thumbStyle := vaxis.Style{
		Foreground: d.scheme.Muted,
		Background: d.scheme.Background,
	}
	for col := bar.Col; col < bar.Col+bar.Length; col++ {
		style := trackStyle
		grapheme := "─"
		if col >= bar.Thumb && col < bar.Thumb+bar.Size {
			style = thumbStyle
			grapheme = horizontalScrollbarThumb
		}
		win.SetCell(col, bar.Row, vaxis.Cell{
			Character: vaxis.Character{
				Grapheme: grapheme,
				Width:    1,
			},
			Style: style,
		})
	}
}

func (d *diffViewer) clampScroll() {
	maxScroll := d.maxScroll()
	if d.scroll < 0 {
		d.scroll = 0
	}
	if d.scroll > maxScroll {
		d.scroll = maxScroll
	}
}

func (d *diffViewer) clampHorizontalScroll() {
	maxScroll := d.maxHorizontalScroll()
	if d.xScroll < 0 {
		d.xScroll = 0
	}
	if d.xScroll > maxScroll {
		d.xScroll = maxScroll
	}
}

func (d *diffViewer) maxScroll() int {
	maxScroll := len(d.rows) - 1
	if visible := d.visibleRowCapacity(); visible > 0 {
		maxScroll = len(d.rows) - visible
	}
	if maxScroll < 0 {
		maxScroll = 0
	}
	return maxScroll
}

func (d *diffViewer) visibleRowCapacity() int {
	_, horizontalVisible := d.scrollbarVisibility(d.width, d.height)
	return visibleRowCapacity(d.height, horizontalVisible)
}

func (d *diffViewer) scrollBy(delta int) {
	d.scroll += delta
	d.clampScroll()
}

func (d *diffViewer) scrollHorizontalBy(delta int) {
	d.xScroll += delta
	d.clampHorizontalScroll()
	d.clampScroll()
}

func (d *diffViewer) scrollTop() {
	d.scroll = 0
}

func (d *diffViewer) scrollBottom() {
	d.scroll = d.maxScroll()
}

func (d *diffViewer) halfPage() int {
	visible := d.visibleRowCapacity()
	if visible < 2 {
		return 1
	}
	return visible / 2
}

func (d *diffViewer) maxHorizontalScroll() int {
	verticalVisible, horizontalVisible := d.scrollbarVisibility(d.width, d.height)
	if !horizontalVisible {
		return 0
	}
	maxScroll := d.contentWidth() - horizontalViewportWidth(d.width, verticalVisible)
	if maxScroll < 0 {
		return 0
	}
	return maxScroll
}

func (d *diffViewer) scrollbarVisibility(width int, height int) (vertical bool, horizontal bool) {
	if width <= 0 || height <= 1 {
		return false, false
	}

	horizontal = d.contentWidth() > width
	vertical = len(d.rows) > visibleRowCapacity(height, horizontal)
	if !horizontal && vertical {
		horizontal = d.contentWidth() > horizontalViewportWidth(width, vertical)
		vertical = len(d.rows) > visibleRowCapacity(height, horizontal)
	}
	return vertical, horizontal
}

func visibleRowCapacity(height int, horizontalVisible bool) int {
	visible := height - 1
	if horizontalVisible {
		visible--
	}
	if visible < 0 {
		return 0
	}
	return visible
}

func horizontalViewportWidth(width int, verticalVisible bool) int {
	if verticalVisible {
		width -= scrollbarWidth
	}
	if width < 0 {
		return 0
	}
	return width
}

func (d *diffViewer) contentWidth() int {
	if d.contentWide > 0 {
		return d.contentWide
	}

	width := 0
	for _, row := range d.rows {
		if rowWidth := textCellWidth(row.Text); rowWidth > width {
			width = rowWidth
		}
	}
	d.contentWide = width
	return width
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

func (d *diffViewer) dimStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Dim,
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

func (d *diffViewer) inlineBackground(kind diff.RowKind) vaxis.Color {
	switch kind {
	case diff.RowAdd:
		return d.scheme.AddInline
	case diff.RowDelete:
		return d.scheme.DeleteInline
	default:
		return vaxis.ColorDefault
	}
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

func printCodeSegmentsAtOffset(win vaxis.Window, col int, row int, offset int, segments ...vaxis.Segment) {
	width, height := win.Size()
	if col >= width || row < 0 || row >= height {
		return
	}

	code := win.New(col, row, -1, 1)
	codeWidth, _ := code.Size()
	paintSegmentsOffset(code, codeWidth, 0, 0, offset, segments...)
}

type cellSetter interface {
	SetCell(col int, row int, cell vaxis.Cell)
}

func paintSegmentsOffset(dst cellSetter, width int, row int, col int, offset int, segments ...vaxis.Segment) {
	paintCol := col - offset
	for _, segment := range segments {
		for _, char := range vaxis.Characters(segment.Text) {
			if paintCol >= width {
				return
			}
			if paintCol >= 0 && char.Width > 0 && paintCol+char.Width <= width {
				dst.SetCell(paintCol, row, vaxis.Cell{
					Character: char,
					Style:     segment.Style,
				})
			}
			paintCol += char.Width
		}
	}
}

func segmentTextWidth(segments []vaxis.Segment) int {
	width := 0
	for _, segment := range segments {
		width += textCellWidth(segment.Text)
	}
	return width
}

func textCellWidth(text string) int {
	width := 0
	for _, char := range vaxis.Characters(text) {
		width += char.Width
	}
	return width
}

func applyInlineSpans(segments []vaxis.Segment, spans []diff.InlineSpan, background vaxis.Color) []vaxis.Segment {
	if len(segments) == 0 || len(spans) == 0 || background == vaxis.ColorDefault {
		return segments
	}

	out := make([]vaxis.Segment, 0, len(segments)+len(spans)*2)
	offset := 0
	spanIndex := 0
	for _, segment := range segments {
		start := offset
		end := offset + len(segment.Text)
		offset = end

		for spanIndex < len(spans) && spans[spanIndex].End <= start {
			spanIndex++
		}

		position := start
		for spanIndex < len(spans) && spans[spanIndex].Start < end {
			span := spans[spanIndex]
			if span.Start > position {
				out = append(out, sliceSegment(segment, position-start, span.Start-start, false, background))
			}

			highlightStart := maxInt(position, span.Start)
			highlightEnd := minInt(end, span.End)
			if highlightStart < highlightEnd {
				out = append(out, sliceSegment(segment, highlightStart-start, highlightEnd-start, true, background))
			}
			position = highlightEnd
			if span.End <= end {
				spanIndex++
			} else {
				break
			}
		}

		if position < end {
			out = append(out, sliceSegment(segment, position-start, end-start, false, background))
		}
	}
	return out
}

func sliceSegment(segment vaxis.Segment, start int, end int, highlight bool, background vaxis.Color) vaxis.Segment {
	style := segment.Style
	if highlight {
		style.Background = background
	}
	return vaxis.Segment{
		Text:  segment.Text[start:end],
		Style: style,
	}
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
