package tui

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"git.sr.ht/~rockorager/vaxis"
	"github.com/rockorager/go-uucode"

	"github.com/rockorager/comview/diff"
)

const pendingKeyTimeout = 800 * time.Millisecond
const multiClickTimeout = 500 * time.Millisecond
const yankHighlightDuration = 180 * time.Millisecond
const mouseWheelScrollLines = 1
const scrollbarWidth = 1
const verticalScrollbarThumb = "█"
const horizontalScrollbarThumb = "\U0001FB0B"
const keyboardFlags = vaxis.CSIuDisambiguate |
	vaxis.CSIuReportEvents |
	vaxis.CSIuAlternateKeys |
	vaxis.CSIuAllKeys |
	vaxis.CSIuAssociatedText

// Run starts the comview TUI.
func Run(input string) error {
	rows, err := rowsForInput(input)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	app, err := NewApp(&diffViewer{
		rows:        rows,
		highlighter: NewSyntaxHighlighter(),
	}, vaxis.Options{
		CSIuBitMask: keyboardFlags,
	})
	if err != nil {
		return err
	}

	return app.Run()
}

func rowsForInput(input string) ([]diff.Row, error) {
	doc, err := diff.Parse(input)
	if err != nil {
		return nil, err
	}
	return doc.RowsWithOptions(diff.DefaultRenderOptions()), nil
}

type diffViewer struct {
	rows         []diff.Row
	scroll       int
	xScroll      int
	height       int
	width        int
	contentWide  int
	codeSegments [][]vaxis.Segment
	fileRows     []int
	cursor       selectionPoint
	cursorGoal   int
	selection    textSelection
	yankUntil    time.Time
	clicks       clickState
	keys         keyChordState
	scheme       ColorScheme
	highlighter  *SyntaxHighlighter
}

type selectionPoint struct {
	Row int
	Col int
}

type textSelection struct {
	Active   bool
	Dragging bool
	Mode     selectionMode
	Anchor   selectionPoint
	Cursor   selectionPoint
}

type selectionMode int

const (
	selectionFull selectionMode = iota
	selectionCode
)

type clickState struct {
	Point selectionPoint
	Mode  selectionMode
	At    time.Time
	Count int
}

func (d *diffViewer) SetTerminalColors(colors TerminalColors) {
	d.ensureColorScheme()
	d.scheme.ApplyTerminalColors(colors)
	if d.highlighter != nil {
		d.highlighter.SetColorScheme(d.scheme)
	}
	d.invalidateRenderCache()
}

func (d *diffViewer) HighlightYank(now time.Time) {
	d.yankUntil = now.Add(yankHighlightDuration)
}

func (d *diffViewer) YankHighlightDuration() time.Duration {
	return yankHighlightDuration
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
	if key.EventType == vaxis.EventRelease {
		return CommandNone, nil
	}

	d.keys.ClearExpired(time.Now())

	switch {
	case key.Matches('c', vaxis.ModCtrl),
		key.Matches('q'),
		key.MatchString("Esc"):
		return CommandQuit, nil
	case key.Matches('g'):
		if d.keys.Pending() == "g" {
			d.keys.Clear()
			d.cursorTop()
			return CommandRedraw, nil
		}
		d.keys.Set("g", time.Now())
		return CommandNone, nil
	case key.Matches('G'), key.Matches(vaxis.KeyEnd):
		d.keys.Clear()
		d.cursorBottom()
		return CommandRedraw, nil
	case key.Matches(vaxis.KeyHome):
		d.keys.Clear()
		d.cursorTop()
		return CommandRedraw, nil
	case key.Matches('d', vaxis.ModCtrl), key.Matches(vaxis.KeyPgDown):
		d.keys.Clear()
		d.moveCursorRows(d.halfPage())
		return CommandRedraw, nil
	case key.Matches('u', vaxis.ModCtrl), key.Matches(vaxis.KeyPgUp):
		d.keys.Clear()
		d.moveCursorRows(-d.halfPage())
		return CommandRedraw, nil
	case key.Matches('j'), key.Matches(vaxis.KeyDown), key.MatchString("Down"):
		d.keys.Clear()
		d.moveCursorRows(1)
		return CommandRedraw, nil
	case key.Matches('k'), key.Matches(vaxis.KeyUp), key.MatchString("Up"):
		d.keys.Clear()
		d.moveCursorRows(-1)
		return CommandRedraw, nil
	case key.Matches('l'), key.Matches(vaxis.KeyRight):
		d.keys.Clear()
		d.moveCursorCols(1)
		return CommandRedraw, nil
	case key.Matches('h'), key.Matches(vaxis.KeyLeft):
		d.keys.Clear()
		d.moveCursorCols(-1)
		return CommandRedraw, nil
	case key.Matches('y'), key.Matches(vaxis.KeyCopy),
		key.Matches('c', vaxis.ModSuper):
		d.keys.Clear()
		if d.ClipboardText() == "" {
			return CommandNone, nil
		}
		return CommandCopy, nil
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
		d.extendSelectionAfterScroll(mouse)
		return CommandRedraw, nil
	case vaxis.MouseWheelUp:
		d.keys.Clear()
		d.scrollBy(-mouseWheelScrollLines)
		d.extendSelectionAfterScroll(mouse)
		return CommandRedraw, nil
	case vaxis.MouseLeftButton:
		switch mouse.EventType {
		case vaxis.EventPress:
			return d.startSelection(mouse), nil
		case vaxis.EventMotion:
			return d.extendSelection(mouse), nil
		case vaxis.EventRelease:
			return d.finishSelection(mouse), nil
		}
	case vaxis.MouseNoButton:
		if mouse.EventType == vaxis.EventRelease {
			return d.finishSelection(mouse), nil
		}
		if mouse.EventType == vaxis.EventMotion {
			return d.extendSelection(mouse), nil
		}
	default:
		return CommandNone, nil
	}
	return CommandNone, nil
}

func (d *diffViewer) Layout(constraints Constraints) Size {
	size := constraints.Constrain(constraints.Max)
	d.height = size.Height
	d.width = size.Width
	d.clampCursor()
	d.clampScroll()
	d.clampHorizontalScroll()
	d.ensureCursorVisible()
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
		docRow := d.scroll + row
		d.printRow(win, row+1, diffRow, d.codeSegments[docRow], docRow == d.cursor.Row)
		d.paintSelection(win, row+1, docRow)
	}
	d.paintStickyFileHeader(win)
	d.paintCursor(win)
	d.paintScrollbar(win)
	d.paintHorizontalScrollbar(win)
}

func (d *diffViewer) paintStickyFileHeader(win vaxis.Window) {
	row, ok := d.stickyFileHeader()
	if !ok {
		return
	}

	d.clearScreenRow(win, 1, d.baseStyle())
	d.printRow(win, 1, row, nil, false)
}

func (d *diffViewer) paintCursor(win vaxis.Window) {
	col, row, ok := d.cursorScreenPosition(win)
	if !ok {
		return
	}
	win.ShowCursor(col, row, vaxis.CursorBlock)
}

func (d *diffViewer) cursorScreenPosition(win vaxis.Window) (int, int, bool) {
	if d.cursor.Row < d.scroll || d.cursor.Row >= len(d.rows) {
		return 0, 0, false
	}

	screenRow := d.cursor.Row - d.scroll + 1
	width, height := win.Size()
	if screenRow < 1 || screenRow >= height {
		return 0, 0, false
	}

	screenCol := d.screenColumn(d.rows[d.cursor.Row], d.cursor.Col)
	if screenCol < 0 || screenCol >= width {
		return 0, 0, false
	}
	return screenCol, screenRow, true
}

func (d *diffViewer) screenColumn(row diff.Row, docCol int) int {
	if row.Code == "" || row.Kind == diff.RowHunk {
		return docCol
	}

	codeOffset := textCellWidth(row.Gutter + row.Marker)
	if docCol < codeOffset {
		return docCol
	}
	return codeOffset + docCol - codeOffset - d.xScroll
}

func (d *diffViewer) stickyFileHeader() (diff.Row, bool) {
	d.ensureFileRows()
	index := sort.Search(len(d.fileRows), func(index int) bool {
		return d.fileRows[index] > d.scroll
	}) - 1
	if index < 0 {
		return diff.Row{}, false
	}

	fileRow := d.fileRows[index]
	if fileRow == d.scroll {
		return diff.Row{}, false
	}
	return d.rows[fileRow], true
}

func (d *diffViewer) ensureFileRows() {
	if d.fileRows != nil {
		return
	}

	d.fileRows = make([]int, 0)
	for index, row := range d.rows {
		if row.Kind == diff.RowFile {
			d.fileRows = append(d.fileRows, index)
		}
	}
}

func (d *diffViewer) printRow(win vaxis.Window, row int, diffRow diff.Row, codeSegments []vaxis.Segment, cursorLine bool) {
	d.fillRowBackground(win, row, diffRow.Kind, cursorLine)
	if diffRow.Kind == diff.RowHunk && diffRow.Prefix != "" && diffRow.Code != "" {
		segments := []vaxis.Segment{
			{Text: diffRow.Prefix, Style: d.rowStyle(d.styleFor(diff.RowHunk), cursorLine)},
			{Text: diffRow.Code, Style: d.rowStyle(d.dimStyle(), cursorLine)},
		}
		printSegmentsAt(win, 0, row, segments...)
		return
	}

	if diffRow.Gutter != "" || diffRow.Marker != "" {
		segments := d.rowSegments(d.gutterSegments(diffRow), cursorLine)
		codeOffset := segmentTextWidth(segments)
		d.fillCodeBackground(win, row, codeOffset, diffRow.Kind, cursorLine)
		printSegmentsAt(win, 0, row, segments...)
		if diffRow.Code != "" {
			printCodeSegmentsAtOffset(win, codeOffset, row, d.xScroll, d.rowSegments(codeSegments, cursorLine)...)
		}
		return
	}

	if diffRow.Code == "" {
		printSegmentsAt(win, 0, row, vaxis.Segment{Text: diffRow.Text, Style: d.rowStyle(d.styleFor(diffRow.Kind), cursorLine)})
		return
	}

	segments := d.rowSegments(d.gutterSegments(diffRow), cursorLine)
	d.fillCodeBackground(win, row, 0, diffRow.Kind, cursorLine)
	printSegmentsAt(win, 0, row, segments...)
	printCodeSegmentsAtOffset(win, 0, row, d.xScroll, d.rowSegments(codeSegments, cursorLine)...)
}

func (d *diffViewer) clearScreenRow(win vaxis.Window, row int, style vaxis.Style) {
	width, height := win.Size()
	if row < 0 || row >= height {
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

func (d *diffViewer) rowSegments(segments []vaxis.Segment, cursorLine bool) []vaxis.Segment {
	if !cursorLine || len(segments) == 0 {
		return segments
	}

	styled := make([]vaxis.Segment, len(segments))
	for i, segment := range segments {
		segment.Style = d.rowStyle(segment.Style, true)
		styled[i] = segment
	}
	return styled
}

func (d *diffViewer) rowStyle(style vaxis.Style, cursorLine bool) vaxis.Style {
	if !cursorLine {
		return style
	}

	background := style.Background
	if background == vaxis.ColorDefault {
		background = d.scheme.Background
	}
	style.Background = blendRGB(background, d.scheme.Foreground, cursorLineBlend)
	return style
}

func (d *diffViewer) fillRowBackground(win vaxis.Window, row int, kind diff.RowKind, cursorLine bool) {
	if !cursorLine && kind != diff.RowAdd && kind != diff.RowDelete {
		return
	}

	style := d.rowStyle(d.styleFor(kind), cursorLine)
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

func (d *diffViewer) fillCodeBackground(win vaxis.Window, row int, start int, kind diff.RowKind, cursorLine bool) {
	width, height := win.Size()
	if row >= height || start >= width {
		return
	}
	if start < 0 {
		start = 0
	}

	style := d.rowStyle(d.codeStyle(kind), cursorLine)
	for col := start; col < width; col++ {
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

	thumbStyle := vaxis.Style{
		Foreground: d.scheme.Muted,
		Background: d.scheme.Background,
	}
	for row := bar.Thumb; row < bar.Thumb+bar.Size; row++ {
		win.SetCell(bar.Col, row, vaxis.Cell{
			Character: vaxis.Character{
				Grapheme: verticalScrollbarThumb,
				Width:    scrollbarWidth,
			},
			Style: thumbStyle,
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

	thumbStyle := vaxis.Style{
		Foreground: d.scheme.Muted,
		Background: d.scheme.Background,
	}
	for col := bar.Thumb; col < bar.Thumb+bar.Size; col++ {
		win.SetCell(col, bar.Row, vaxis.Cell{
			Character: vaxis.Character{
				Grapheme: horizontalScrollbarThumb,
				Width:    1,
			},
			Style: thumbStyle,
		})
	}
}

func (d *diffViewer) startSelection(mouse vaxis.Mouse) Command {
	mode := selectionFull
	if mouse.Modifiers&vaxis.ModAlt != 0 {
		mode = selectionCode
	}

	point, ok := d.selectionPointForMode(mouse, mode)
	if !ok {
		if d.selection.Active {
			d.selection = textSelection{}
			return CommandRedraw
		}
		return CommandNone
	}

	d.keys.Clear()
	d.setCursor(point)
	switch d.registerClick(point, mode, time.Now()) {
	case 2:
		return d.selectToken(point, mode)
	case 3:
		return d.selectRow(point.Row, mode)
	}

	d.selection = textSelection{
		Active:   true,
		Dragging: true,
		Mode:     mode,
		Anchor:   point,
		Cursor:   point,
	}
	return CommandRedraw
}

func (d *diffViewer) registerClick(point selectionPoint, mode selectionMode, now time.Time) int {
	if d.clicks.Point == point && d.clicks.Mode == mode && now.Sub(d.clicks.At) <= multiClickTimeout {
		d.clicks.Count++
	} else {
		d.clicks.Count = 1
	}
	if d.clicks.Count > 3 {
		d.clicks.Count = 1
	}
	d.clicks.Point = point
	d.clicks.Mode = mode
	d.clicks.At = now
	return d.clicks.Count
}

func (d *diffViewer) selectToken(point selectionPoint, mode selectionMode) Command {
	if point.Row < 0 || point.Row >= len(d.rows) {
		return CommandNone
	}

	start, end := tokenRangeAt(d.rows[point.Row].Text, point.Col)
	if mode == selectionCode {
		codeStart, codeEnd, ok := codeRange(d.rows[point.Row])
		if !ok {
			return CommandNone
		}
		start = maxInt(start, codeStart)
		end = minInt(end, codeEnd)
	}
	d.selection = textSelection{
		Active: true,
		Mode:   mode,
		Anchor: selectionPoint{
			Row: point.Row,
			Col: start,
		},
		Cursor: selectionPoint{
			Row: point.Row,
			Col: maxInt(start, end-1),
		},
	}
	d.setCursor(point)
	return CommandRedraw
}

func (d *diffViewer) selectRow(row int, mode selectionMode) Command {
	if row < 0 || row >= len(d.rows) {
		return CommandNone
	}

	start := 0
	end := textCellWidth(d.rows[row].Text)
	if mode == selectionCode {
		var ok bool
		start, end, ok = codeRange(d.rows[row])
		if !ok {
			return CommandNone
		}
	}
	d.selection = textSelection{
		Active: true,
		Mode:   mode,
		Anchor: selectionPoint{
			Row: row,
			Col: start,
		},
		Cursor: selectionPoint{
			Row: row,
			Col: maxInt(start, end-1),
		},
	}
	d.setCursor(selectionPoint{Row: row, Col: start})
	return CommandRedraw
}

func (d *diffViewer) extendSelection(mouse vaxis.Mouse) Command {
	if !d.selection.Dragging {
		return CommandNone
	}
	point, ok := d.selectionPointForMode(mouse, d.selection.Mode)
	if !ok {
		return CommandNone
	}

	d.selection.Cursor = point
	d.setCursor(point)
	return CommandRedraw
}

func (d *diffViewer) extendSelectionAfterScroll(mouse vaxis.Mouse) {
	if !d.selection.Dragging {
		return
	}
	point, ok := d.selectionPointForMode(mouse, d.selection.Mode)
	if !ok {
		return
	}
	d.selection.Cursor = point
	d.setCursor(point)
}

func (d *diffViewer) finishSelection(mouse vaxis.Mouse) Command {
	if !d.selection.Dragging {
		return CommandNone
	}
	point, ok := d.selectionPointForMode(mouse, d.selection.Mode)
	if ok {
		d.selection.Cursor = point
		d.setCursor(point)
	}
	d.selection.Dragging = false
	return CommandRedraw
}

func (d *diffViewer) selectionPoint(mouse vaxis.Mouse) (selectionPoint, bool) {
	return d.selectionPointForMode(mouse, selectionFull)
}

func (d *diffViewer) selectionPointForMode(mouse vaxis.Mouse, mode selectionMode) (selectionPoint, bool) {
	row := d.scroll + mouse.Row - 1
	if mouse.Row <= 0 || row < 0 || row >= len(d.rows) {
		return selectionPoint{}, false
	}

	docCol := d.documentColumn(d.rows[row], mouse.Col)
	start := 0
	end := textCellWidth(d.rows[row].Text)
	if mode == selectionCode {
		var ok bool
		start, end, ok = codeRange(d.rows[row])
		if !ok {
			return selectionPoint{}, false
		}
	}
	if docCol < start {
		docCol = start
	}
	if docCol > end {
		docCol = end
	}
	return selectionPoint{Row: row, Col: docCol}, true
}

func (d *diffViewer) documentColumn(row diff.Row, screenCol int) int {
	if screenCol < 0 {
		return 0
	}
	if row.Code == "" || row.Kind == diff.RowHunk {
		return screenCol
	}

	codeOffset := textCellWidth(row.Gutter + row.Marker)
	if screenCol < codeOffset {
		return screenCol
	}
	return codeOffset + d.xScroll + screenCol - codeOffset
}

func codeRange(row diff.Row) (int, int, bool) {
	switch {
	case row.Kind == diff.RowHunk && row.Code != "":
		start := textCellWidth(row.Prefix)
		return start, start + textCellWidth(row.Code), true
	case row.Gutter != "" || row.Marker != "":
		start := textCellWidth(row.Gutter + row.Marker)
		return start, start + textCellWidth(row.Code), true
	case row.Code != "":
		return 0, textCellWidth(row.Code), true
	default:
		return 0, 0, false
	}
}

func (d *diffViewer) paintSelection(win vaxis.Window, screenRow int, docRow int) {
	start, end, ok := d.selectionRange()
	if !ok || docRow < start.Row || docRow > end.Row {
		return
	}

	startCol, endCol, ok := d.selectionPaintRange(docRow, start, end)
	if !ok {
		return
	}

	row := d.rows[docRow]
	width, _ := win.Size()
	if startCol >= width {
		return
	}
	if endCol > width && (row.Code == "" || row.Kind == diff.RowHunk) {
		endCol = width
	}

	style := d.selectionStyleAt(time.Now())
	for screenCol := 0; screenCol < width; screenCol++ {
		docCol := d.documentColumn(row, screenCol)
		if docCol >= startCol && docCol < endCol {
			win.SetStyle(screenCol, screenRow, style)
		}
	}
}

func (d *diffViewer) selectionPaintRange(docRow int, start selectionPoint, end selectionPoint) (int, int, bool) {
	startCol, endCol, ok := d.selectionRenderRange(docRow, start, end)
	if !ok {
		return 0, 0, false
	}
	if startCol >= endCol {
		endCol = startCol + 1
	}
	return startCol, endCol, true
}

func (d *diffViewer) selectionRenderRange(docRow int, start selectionPoint, end selectionPoint) (int, int, bool) {
	row := d.rows[docRow]
	startCol := 0
	endCol := textCellWidth(row.Text)
	if d.selection.Mode == selectionCode {
		var ok bool
		startCol, endCol, ok = codeRange(row)
		if !ok {
			return 0, 0, false
		}
	}
	if docRow == start.Row {
		startCol = maxInt(startCol, start.Col)
	}
	if docRow == end.Row {
		endCol = minInt(endCol, end.Col)
	}
	return startCol, endCol, true
}

func (d *diffViewer) selectionRange() (selectionPoint, selectionPoint, bool) {
	if !d.selection.Active {
		return selectionPoint{}, selectionPoint{}, false
	}

	start := d.selection.Anchor
	end := d.selection.Cursor
	if selectionPointLess(end, start) {
		start, end = end, start
	}
	end.Col++
	return start, end, true
}

func selectionPointLess(a selectionPoint, b selectionPoint) bool {
	if a.Row != b.Row {
		return a.Row < b.Row
	}
	return a.Col < b.Col
}

func (d *diffViewer) ClipboardText() string {
	start, end, ok := d.selectionRange()
	if !ok {
		return ""
	}

	var text strings.Builder
	for rowIndex := start.Row; rowIndex <= end.Row && rowIndex < len(d.rows); rowIndex++ {
		rowStart := 0
		rowEnd := textCellWidth(d.rows[rowIndex].Text)
		if d.selection.Mode == selectionCode {
			var ok bool
			rowStart, rowEnd, ok = codeRange(d.rows[rowIndex])
			if !ok {
				continue
			}
		}
		if rowIndex == start.Row {
			rowStart = maxInt(rowStart, start.Col)
		}
		if rowIndex == end.Row {
			rowEnd = minInt(rowEnd, end.Col)
		}
		if rowStart < rowEnd {
			text.WriteString(cellTextRange(d.rows[rowIndex].Text, rowStart, rowEnd))
		}
		if rowIndex != end.Row {
			text.WriteByte('\n')
		}
	}
	return text.String()
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

func (d *diffViewer) clampCursor() {
	if len(d.rows) == 0 {
		d.cursor = selectionPoint{}
		d.cursorGoal = 0
		return
	}

	if d.cursor.Row < 0 {
		d.cursor.Row = 0
	}
	if d.cursor.Row >= len(d.rows) {
		d.cursor.Row = len(d.rows) - 1
	}
	d.cursor.Col = d.clampCursorCol(d.cursor.Row, d.cursor.Col)
}

func (d *diffViewer) clampCursorCol(row int, col int) int {
	if row < 0 || row >= len(d.rows) {
		return 0
	}
	if col < 0 {
		return 0
	}
	if width := textCellWidth(d.rows[row].Text); col > width {
		return width
	}
	return col
}

func (d *diffViewer) setCursor(point selectionPoint) {
	d.cursor = point
	d.clampCursor()
	d.cursorGoal = d.cursor.Col
	d.ensureCursorVisible()
}

func (d *diffViewer) moveCursorRows(delta int) {
	d.prepareCursorForMovement()
	d.cursor.Row += delta
	d.clampCursor()
	d.cursor.Col = d.clampCursorCol(d.cursor.Row, d.cursorGoal)
	d.ensureCursorVisible()
}

func (d *diffViewer) moveCursorCols(delta int) {
	d.prepareCursorForMovement()
	d.cursor.Col += delta
	d.clampCursor()
	d.cursorGoal = d.cursor.Col
	d.ensureCursorVisible()
}

func (d *diffViewer) cursorTop() {
	d.cursor.Row = 0
	d.clampCursor()
	d.cursorGoal = d.cursor.Col
	d.ensureCursorVisible()
}

func (d *diffViewer) cursorBottom() {
	d.cursor.Row = len(d.rows) - 1
	d.clampCursor()
	d.cursorGoal = d.cursor.Col
	d.ensureCursorVisible()
}

func (d *diffViewer) prepareCursorForMovement() {
	if len(d.rows) == 0 {
		return
	}
	d.clampCursor()
	visible := d.visibleRowCapacity()
	if visible <= 0 {
		return
	}
	if d.cursor.Row < d.scroll || d.cursor.Row >= d.scroll+visible {
		d.cursor.Row = d.scroll
		d.clampCursor()
		d.cursorGoal = d.cursor.Col
	}
}

func (d *diffViewer) ensureCursorVisible() {
	d.clampCursor()
	if len(d.rows) == 0 {
		return
	}

	visible := d.visibleRowCapacity()
	if visible > 0 {
		if d.cursor.Row < d.scroll {
			d.scroll = d.cursor.Row
		}
		if d.cursor.Row >= d.scroll+visible {
			d.scroll = d.cursor.Row - visible + 1
		}
		d.clampScroll()
		if _, ok := d.stickyFileHeader(); ok && d.cursor.Row == d.scroll && d.scroll > 0 {
			d.scroll--
		}
		d.clampScroll()
	}

	d.ensureCursorColumnVisible()
}

func (d *diffViewer) ensureCursorColumnVisible() {
	if d.cursor.Row < 0 || d.cursor.Row >= len(d.rows) {
		return
	}

	row := d.rows[d.cursor.Row]
	if row.Code == "" || row.Kind == diff.RowHunk {
		return
	}

	codeOffset := textCellWidth(row.Gutter + row.Marker)
	if d.cursor.Col < codeOffset {
		return
	}

	verticalVisible, _ := d.scrollbarVisibility(d.width, d.height)
	viewportWidth := horizontalViewportWidth(d.width, verticalVisible)
	codeWidth := viewportWidth - codeOffset
	if codeWidth < 1 {
		codeWidth = 1
	}

	codeCol := d.cursor.Col - codeOffset
	if codeCol < d.xScroll {
		d.xScroll = codeCol
	}
	if codeCol >= d.xScroll+codeWidth {
		d.xScroll = codeCol - codeWidth + 1
	}
	d.clampHorizontalScroll()
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
	screenOffset := d.cursor.Row - d.scroll
	if visible := d.visibleRowCapacity(); screenOffset < 0 || screenOffset >= visible {
		screenOffset = 0
	}

	d.scroll += delta
	d.clampScroll()
	d.cursor.Row = d.scroll + screenOffset
	if _, ok := d.stickyFileHeader(); ok && screenOffset == 0 && d.cursor.Row+1 < len(d.rows) {
		d.cursor.Row++
	}
	d.clampCursor()
	d.cursorGoal = d.cursor.Col
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
	style.Background = d.scheme.Code
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
		Background: d.scheme.Gutter,
	}
	switch kind {
	case diff.RowAdd:
		style.Foreground = d.scheme.Add
	case diff.RowDelete:
		style.Foreground = d.scheme.Delete
	}
	return style
}

func (d *diffViewer) gutterSegments(row diff.Row) []vaxis.Segment {
	return []vaxis.Segment{{Text: row.Gutter + row.Marker, Style: d.gutterStyle(row.Kind)}}
}

func (d *diffViewer) selectionStyle() vaxis.Style {
	return d.selectionStyleAt(time.Now())
}

func (d *diffViewer) selectionStyleAt(now time.Time) vaxis.Style {
	background := d.scheme.Selection
	if !d.yankUntil.IsZero() && now.Before(d.yankUntil) {
		background = d.scheme.Yank
	}
	return vaxis.Style{
		Foreground: d.scheme.Foreground,
		Background: background,
	}
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

func cellTextRange(text string, start int, end int) string {
	if start < 0 {
		start = 0
	}
	if end <= start {
		return ""
	}

	var out strings.Builder
	col := 0
	for _, char := range vaxis.Characters(text) {
		next := col + char.Width
		if next > start && col < end {
			out.WriteString(char.Grapheme)
		}
		col = next
		if col >= end {
			break
		}
	}
	return out.String()
}

type textCell struct {
	Start int
	End   int
	Kind  int
}

const (
	spaceSelectionToken = iota + 1
	wordSelectionToken
	punctuationSelectionToken
	symbolSelectionToken
)

func tokenRangeAt(text string, col int) (int, int) {
	cells := textCells(text)
	if len(cells) == 0 {
		return 0, 0
	}

	index := len(cells) - 1
	for i, cell := range cells {
		if col < cell.End {
			index = i
			break
		}
	}

	kind := cells[index].Kind
	start := index
	for start > 0 && cells[start-1].Kind == kind {
		start--
	}
	end := index + 1
	for end < len(cells) && cells[end].Kind == kind {
		end++
	}
	return cells[start].Start, cells[end-1].End
}

func textCells(text string) []textCell {
	chars := vaxis.Characters(text)
	cells := make([]textCell, 0, len(chars))
	col := 0
	for _, char := range chars {
		start := col
		end := start + char.Width
		col = end
		if char.Width <= 0 {
			continue
		}
		cells = append(cells, textCell{
			Start: start,
			End:   end,
			Kind:  selectionTokenKind(char.Grapheme),
		})
	}
	return cells
}

func selectionTokenKind(text string) int {
	r, _ := utf8.DecodeRuneInString(text)
	switch {
	case uucode.IsSpace(r):
		return spaceSelectionToken
	case uucode.IsLetter(r) || uucode.IsDigit(r) || r == '_':
		return wordSelectionToken
	case uucode.IsPunct(r):
		return punctuationSelectionToken
	default:
		return symbolSelectionToken
	}
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
