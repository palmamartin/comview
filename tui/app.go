package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"git.sr.ht/~rockorager/vaxis"
	"github.com/rockorager/go-uucode"

	"github.com/rockorager/comview/diff"
	"github.com/rockorager/comview/review"
)

const pendingKeyTimeout = 800 * time.Millisecond
const multiClickTimeout = 500 * time.Millisecond
const yankHighlightDuration = 180 * time.Millisecond
const mouseWheelScrollLines = 1
const scrollbarWidth = 1
const commentEditorRows = 3
const commentEditorBoxHeight = commentEditorRows + 2
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

	commentFile, err := review.LoadFile(review.DefaultFilePath)
	if err != nil {
		return err
	}
	app, err := NewApp(&diffViewer{
		rows:         rows,
		reviewDrafts: commentFile.Comments,
		reviewFile:   review.DefaultFilePath,
		highlighter:  NewSyntaxHighlighter(),
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
	rows          []diff.Row
	scroll        int
	xScroll       int
	height        int
	width         int
	contentWide   int
	codeSegments  [][]vaxis.Segment
	fileRows      []int
	cursor        selectionPoint
	cursorGoal    int
	mode          viewMode
	selection     textSelection
	yankSelection textSelection
	clipboardText string
	reviewDrafts  []review.CommentDraft
	reviewDirty   bool
	reviewFile    string
	editor        *commentEditor
	commandLine   string
	searchQuery   string
	searchMatches []searchMatch
	searchIndex   int
	statusMessage string
	yankUntil     time.Time
	mouseDrag     mouseDragState
	clicks        clickState
	keys          keyChordState
	scheme        ColorScheme
	highlighter   *SyntaxHighlighter
}

type selectionPoint struct {
	Row int
	Col int
}

type searchMatch struct {
	Row   int
	Start int
	End   int
}

type textSelection struct {
	Active   bool
	Dragging bool
	Anchor   selectionPoint
	Cursor   selectionPoint
}

type commentEditor struct {
	draft      review.CommentDraft
	draftIndex int
	lines      []string
	row        int
	col        int
}

type mouseDragState struct {
	Active    bool
	Started   bool
	Mouse     vaxis.Mouse
	Row       int
	Anchor    selectionPoint
	HasAnchor bool
}

type viewMode int

const (
	modeNormal viewMode = iota
	modeVisual
	modeVisualLine
	modeInsert
	modeCommand
	modeSearch
)

type clickState struct {
	Point selectionPoint
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
	if d.selection.Active {
		d.yankSelection = d.selection
	}
	d.yankUntil = now.Add(yankHighlightDuration)
}

func (d *diffViewer) YankHighlightDuration() time.Duration {
	return yankHighlightDuration
}

func (d *diffViewer) ClipboardConsumed() {
	d.clipboardText = ""
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

	if d.mode == modeCommand {
		return d.handleCommandKey(key), nil
	}
	if d.mode == modeSearch {
		return d.handleSearchKey(key)
	}
	if d.mode == modeInsert && d.editor != nil {
		return d.handleCommentKey(key), nil
	}

	d.keys.ClearExpired(time.Now())

	switch {
	case key.Matches('/'):
		d.keys.Clear()
		d.enterSearchMode()
		return CommandRedraw, nil
	case key.Matches(':'):
		d.keys.Clear()
		d.enterCommandMode()
		return CommandRedraw, nil
	case key.Matches('N'):
		d.keys.Clear()
		if !d.moveSearchMatch(-1) {
			return CommandNone, nil
		}
		return CommandRedraw, nil
	case key.Matches('n'):
		d.keys.Clear()
		if !d.moveSearchMatch(1) {
			return CommandNone, nil
		}
		return CommandRedraw, nil
	case key.Matches('c', vaxis.ModCtrl), key.Matches('q'):
		d.keys.Clear()
		return CommandNone, nil
	case key.Matches(vaxis.KeyEsc), key.MatchString("Esc"):
		if d.mode != modeNormal || d.selection.Active {
			d.keys.Clear()
			d.exitVisualMode()
			return CommandRedraw, nil
		}
		if d.clearSearch() {
			d.keys.Clear()
			return CommandRedraw, nil
		}
		return CommandNone, nil
	case key.Matches('v'):
		d.keys.Clear()
		if d.mode == modeVisual {
			d.exitVisualMode()
		} else {
			d.enterVisualMode()
		}
		return CommandRedraw, nil
	case key.Matches('V'):
		d.keys.Clear()
		if d.mode == modeVisualLine {
			d.exitVisualMode()
		} else {
			d.enterVisualLineMode()
		}
		return CommandRedraw, nil
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
	case key.Matches('i'):
		d.keys.Clear()
		if d.editor != nil {
			d.mode = modeInsert
			return CommandRedraw, nil
		}
		if !d.openReviewCommentEditor() {
			return CommandNone, nil
		}
		return CommandRedraw, nil
	case key.Matches('y'), key.Matches(vaxis.KeyCopy),
		key.Matches('c', vaxis.ModSuper):
		d.keys.Clear()
		if !d.prepareYank() {
			return CommandNone, nil
		}
		return CommandCopy, nil
	default:
		d.keys.Clear()
		return CommandNone, nil
	}
}

func (d *diffViewer) handleMouse(mouse vaxis.Mouse) (Command, error) {
	if d.mode == modeInsert || d.mode == modeCommand || d.mode == modeSearch {
		return CommandNone, nil
	}

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

	if len(d.rows) == 0 {
		printAt(win, 0, 0, "Pipe git diff or git show into comview.", d.baseStyle())
		printAt(win, 0, 2, "Press q, Esc, Ctrl+C, or Ctrl+D to quit.", mutedStyle)
		d.paintStatusBar(win)
		return
	}

	for row, diffRow := range d.visibleRows() {
		docRow := d.scroll + row
		d.printRow(win, row, docRow, diffRow, d.codeSegments[docRow], docRow == d.cursor.Row)
		d.paintSelection(win, row, docRow)
	}
	d.paintStickyFileHeader(win)
	d.paintCursor(win)
	d.paintScrollbar(win)
	d.paintHorizontalScrollbar(win)
	d.paintCommentEditor(win)
	d.paintStatusBar(win)
}

func (d *diffViewer) paintStickyFileHeader(win vaxis.Window) {
	row, ok := d.stickyFileHeader()
	if !ok {
		return
	}

	d.clearScreenRow(win, 0, d.baseStyle())
	d.printRow(win, 0, -1, row, nil, false)
}

func (d *diffViewer) paintCursor(win vaxis.Window) {
	if win.Vx != nil {
		win.Vx.HideCursor()
	}
	if d.mode == modeCommand {
		return
	}

	col, row, ok := d.cursorScreenPosition(win)
	if !ok {
		return
	}
	win.SetCell(col, row, vaxis.Cell{
		Character: characterAtCell(d.rows[d.cursor.Row].Text, d.cursor.Col),
		Style:     d.cursorCellStyle(),
	})
}

func (d *diffViewer) cursorScreenPosition(win vaxis.Window) (int, int, bool) {
	width, height := win.Size()
	return d.cursorScreenPositionForSize(width, height)
}

func (d *diffViewer) cursorScreenPositionForSize(width int, height int) (int, int, bool) {
	if d.cursor.Row < d.scroll || d.cursor.Row >= len(d.rows) {
		return 0, 0, false
	}

	screenRow := d.cursor.Row - d.scroll
	if screenRow < 0 || screenRow >= d.visibleRowCapacity() || screenRow >= height {
		return 0, 0, false
	}

	screenCol := d.screenColumn(d.rows[d.cursor.Row], d.cursor.Col)
	if screenCol < 0 || screenCol >= width {
		return 0, 0, false
	}
	return screenCol, screenRow, true
}

func (d *diffViewer) cursorCellStyle() vaxis.Style {
	if d.cursor.Row < 0 || d.cursor.Row >= len(d.rows) {
		return d.reverseStyle(d.baseStyle())
	}

	row := d.rows[d.cursor.Row]
	style := d.cursorBaseStyle(row)
	return d.reverseStyle(d.rowStyle(style, true))
}

func (d *diffViewer) cursorBaseStyle(row diff.Row) vaxis.Style {
	style := d.styleFor(row.Kind)
	switch {
	case row.Kind == diff.RowHunk && row.Prefix != "" && row.Code != "":
		if d.cursor.Col >= textCellWidth(row.Prefix) {
			style = d.dimStyle()
		}
	case row.Gutter != "" || row.Marker != "":
		codeStart := d.codeOffset(row)
		if d.cursor.Col < codeStart {
			style = d.gutterStyle(row.Kind)
		} else if segmentStyle, ok := segmentStyleAt(d.codeSegmentsForRow(d.cursor.Row), d.cursor.Col-codeStart); ok {
			style = segmentStyle
		} else {
			style = d.codeStyle(row.Kind)
		}
	case row.Code != "":
		if segmentStyle, ok := segmentStyleAt(d.codeSegmentsForRow(d.cursor.Row), d.cursor.Col); ok {
			style = segmentStyle
		} else {
			style = d.codeStyle(row.Kind)
		}
	}
	return style
}

func (d *diffViewer) codeOffset(row diff.Row) int {
	return textCellWidth(row.Gutter + row.Marker)
}

func (d *diffViewer) codeSegmentsForRow(row int) []vaxis.Segment {
	if row < 0 || row >= len(d.codeSegments) {
		return nil
	}
	return d.codeSegments[row]
}

func (d *diffViewer) reverseStyle(style vaxis.Style) vaxis.Style {
	foreground := style.Foreground
	background := style.Background
	if foreground == vaxis.ColorDefault {
		foreground = d.scheme.Foreground
	}
	if background == vaxis.ColorDefault {
		background = d.scheme.Background
	}
	style.Foreground = background
	style.Background = foreground
	return style
}

func (d *diffViewer) paintStatusBar(win vaxis.Window) {
	width, height := win.Size()
	if width <= 0 || height <= 0 {
		return
	}

	fillStyle := d.statusFillStyle()
	row := height - 1
	for col := 0; col < width; col++ {
		win.SetCell(col, row, vaxis.Cell{
			Character: vaxis.Character{
				Grapheme: " ",
				Width:    1,
			},
			Style: fillStyle,
		})
	}
	if d.mode == modeCommand {
		printSegmentsAt(win, 0, row, vaxis.Segment{
			Text:  ":" + d.commandLine,
			Style: d.baseStyle(),
		})
		d.paintCommandCursor(win)
		return
	}
	if d.mode == modeSearch {
		printSegmentsAt(win, 0, row, vaxis.Segment{
			Text:  "/" + d.searchQuery,
			Style: d.baseStyle(),
		})
		d.paintSearchCursor(win)
		return
	}
	printSegmentsAt(win, 0, row,
		vaxis.Segment{
			Text:  " " + d.modeLabel() + " ",
			Style: d.statusStyle(),
		},
		vaxis.Segment{
			Text:  "",
			Style: d.statusSeparatorStyle(),
		},
	)
	if d.statusMessage != "" {
		printSegmentsAt(win, textCellWidth(" "+d.modeLabel()+"  "), row, vaxis.Segment{
			Text:  d.statusMessage,
			Style: d.dimStyle(),
		})
	}
}

func (d *diffViewer) paintCommandCursor(win vaxis.Window) {
	col, row, ok := d.commandCursorPosition(win)
	if !ok {
		return
	}
	win.SetCell(col, row, vaxis.Cell{
		Character: vaxis.Character{Grapheme: " ", Width: 1},
		Style:     d.reverseStyle(d.baseStyle()),
	})
}

func (d *diffViewer) paintSearchCursor(win vaxis.Window) {
	col, row, ok := d.searchCursorPosition(win)
	if !ok {
		return
	}
	win.SetCell(col, row, vaxis.Cell{
		Character: vaxis.Character{Grapheme: " ", Width: 1},
		Style:     d.reverseStyle(d.baseStyle()),
	})
}

func (d *diffViewer) commandCursorPosition(win vaxis.Window) (int, int, bool) {
	width, height := win.Size()
	return d.commandCursorPositionForSize(width, height)
}

func (d *diffViewer) commandCursorPositionForSize(width int, height int) (int, int, bool) {
	if width <= 0 || height <= 0 || d.mode != modeCommand {
		return 0, 0, false
	}
	col := 1 + textCellWidth(d.commandLine)
	if col >= width {
		col = width - 1
	}
	return col, height - 1, true
}

func (d *diffViewer) searchCursorPosition(win vaxis.Window) (int, int, bool) {
	width, height := win.Size()
	if width <= 0 || height <= 0 || d.mode != modeSearch {
		return 0, 0, false
	}
	col := 1 + textCellWidth(d.searchQuery)
	if col >= width {
		col = width - 1
	}
	return col, height - 1, true
}

func (d *diffViewer) paintCommentEditor(win vaxis.Window) {
	if d.editor == nil {
		return
	}

	width, height := win.Size()
	x, y, boxWidth, boxHeight, ok := d.commentEditorRect(width, height)
	if !ok {
		return
	}

	inputStyle := vaxis.Style{
		Foreground: d.scheme.Foreground,
		Background: blendRGB(d.scheme.Background, d.scheme.Foreground, 0.08),
	}
	borderStyle := inputStyle
	borderStyle.Foreground = d.scheme.Muted

	d.paintCommentBorder(win, x, y, boxWidth, boxHeight, borderStyle)
	for row := 0; row < commentEditorRows; row++ {
		screenRow := y + 1 + row
		for col := 0; col < boxWidth-2; col++ {
			win.SetCell(x+1+col, screenRow, vaxis.Cell{
				Character: vaxis.Character{Grapheme: " ", Width: 1},
				Style:     inputStyle,
			})
		}
		if row < len(d.editor.lines) {
			printSegmentsAt(win.New(x+1, screenRow, boxWidth-2, 1), 0, 0, vaxis.Segment{
				Text:  d.editor.lines[row],
				Style: inputStyle,
			})
		}
	}

	d.paintCommentCursor(win, x+1, y+1, boxWidth-2, inputStyle)
}

func (d *diffViewer) commentEditorRect(width int, height int) (int, int, int, int, bool) {
	if d.editor == nil || width <= 0 || height <= 1 || commentEditorBoxHeight > height {
		return 0, 0, 0, 0, false
	}

	screenCol, screenRow, ok := d.cursorScreenPositionForSize(width, height)
	if !ok {
		return 0, 0, 0, 0, false
	}

	verticalVisible, _ := d.scrollbarVisibility(width, height)
	viewportWidth := horizontalViewportWidth(width, verticalVisible)
	boxWidth := minInt(viewportWidth, 72)
	if viewportWidth > 4 {
		boxWidth = minInt(viewportWidth-4, 72)
	}
	if boxWidth < 4 {
		boxWidth = viewportWidth
	}
	if boxWidth < 3 {
		return 0, 0, 0, 0, false
	}

	x := screenCol
	if x+boxWidth > viewportWidth {
		x = viewportWidth - boxWidth
	}
	if x < 0 {
		x = 0
	}
	y := screenRow + 1
	return x, y, boxWidth, commentEditorBoxHeight, true
}

func (d *diffViewer) paintCommentBorder(win vaxis.Window, x int, y int, width int, height int, style vaxis.Style) {
	if width < 2 || height < 2 {
		return
	}

	right := x + width - 1
	bottom := y + height - 1
	win.SetCell(x, y, vaxis.Cell{Character: vaxis.Character{Grapheme: "┌", Width: 1}, Style: style})
	win.SetCell(right, y, vaxis.Cell{Character: vaxis.Character{Grapheme: "┐", Width: 1}, Style: style})
	win.SetCell(x, bottom, vaxis.Cell{Character: vaxis.Character{Grapheme: "└", Width: 1}, Style: style})
	win.SetCell(right, bottom, vaxis.Cell{Character: vaxis.Character{Grapheme: "┘", Width: 1}, Style: style})
	for col := x + 1; col < right; col++ {
		win.SetCell(col, y, vaxis.Cell{Character: vaxis.Character{Grapheme: "─", Width: 1}, Style: style})
		win.SetCell(col, bottom, vaxis.Cell{Character: vaxis.Character{Grapheme: "─", Width: 1}, Style: style})
	}
	for row := y + 1; row < bottom; row++ {
		win.SetCell(x, row, vaxis.Cell{Character: vaxis.Character{Grapheme: "│", Width: 1}, Style: style})
		win.SetCell(right, row, vaxis.Cell{Character: vaxis.Character{Grapheme: "│", Width: 1}, Style: style})
	}
}

func (d *diffViewer) paintCommentCursor(win vaxis.Window, x int, y int, width int, style vaxis.Style) {
	if d.editor.row < 0 || d.editor.row >= commentEditorRows || d.editor.col < 0 || d.editor.col >= width {
		return
	}
	line := ""
	if d.editor.row < len(d.editor.lines) {
		line = d.editor.lines[d.editor.row]
	}
	char := runeAtIndex(line, d.editor.col)
	win.SetCell(x+d.editor.col, y+d.editor.row, vaxis.Cell{
		Character: vaxis.Character{Grapheme: string(char), Width: 1},
		Style:     d.reverseStyle(style),
	})
}

func (d *diffViewer) statusFillStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Foreground,
		Background: d.scheme.Background,
	}
}

func (d *diffViewer) statusStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Background,
		Background: d.statusColor(),
		Attribute:  vaxis.AttrBold,
	}
}

func (d *diffViewer) statusSeparatorStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.statusColor(),
		Background: d.scheme.Background,
	}
}

func (d *diffViewer) statusColor() vaxis.Color {
	switch d.mode {
	case modeVisual, modeVisualLine:
		return d.scheme.Base.Magenta
	case modeInsert:
		return d.scheme.Base.Green
	default:
		return d.scheme.Base.Blue
	}
}

func (d *diffViewer) modeLabel() string {
	switch d.mode {
	case modeVisual:
		return "VISUAL"
	case modeVisualLine:
		return "V-LINE"
	case modeInsert:
		return "INSERT"
	case modeCommand:
		return "COMMAND"
	case modeSearch:
		return "SEARCH"
	default:
		return "NORMAL"
	}
}

func (d *diffViewer) screenColumn(row diff.Row, docCol int) int {
	if row.Code == "" || row.Kind == diff.RowHunk {
		return docCol
	}

	codeOffset := d.codeOffset(row)
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

func (d *diffViewer) printRow(win vaxis.Window, row int, docRow int, diffRow diff.Row, codeSegments []vaxis.Segment, cursorLine bool) {
	d.fillRowBackground(win, row, diffRow.Kind, cursorLine)
	if diffRow.Prefix != "" && diffRow.Code != "" && d.printStructuredRow(win, row, docRow, diffRow, cursorLine) {
		return
	}

	if diffRow.Gutter != "" || diffRow.Marker != "" {
		segments := d.rowSegments(d.gutterSegments(diffRow), cursorLine)
		codeOffset := segmentTextWidth(segments)
		d.fillCodeBackground(win, row, codeOffset, diffRow.Kind, cursorLine)
		printSegmentsAt(win, 0, row, segments...)
		if diffRow.Code != "" {
			codeSegments = d.reviewSegments(diffRow, codeSegments)
			codeSegments = d.searchSegments(docRow, diffRow, codeSegments)
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
	codeSegments = d.reviewSegments(diffRow, codeSegments)
	codeSegments = d.searchSegments(docRow, diffRow, codeSegments)
	printCodeSegmentsAtOffset(win, 0, row, d.xScroll, d.rowSegments(codeSegments, cursorLine)...)
}

func (d *diffViewer) printStructuredRow(win vaxis.Window, row int, docRow int, diffRow diff.Row, cursorLine bool) bool {
	segments, ok := d.structuredSegments(diffRow)
	if !ok {
		return false
	}
	segments = d.searchSegments(docRow, diffRow, segments)
	printSegmentsAt(win, 0, row, d.rowSegments(segments, cursorLine)...)
	return true
}

func (d *diffViewer) structuredSegments(row diff.Row) ([]vaxis.Segment, bool) {
	switch row.Kind {
	case diff.RowHunk:
		return []vaxis.Segment{
			{Text: row.Prefix, Style: d.styleFor(diff.RowHunk)},
			{Text: row.Code, Style: d.dimStyle()},
		}, true
	case diff.RowCommitHeader:
		return []vaxis.Segment{
			{Text: row.Prefix, Style: d.dimStyle()},
			{Text: row.Code, Style: d.commitHashStyle()},
		}, true
	case diff.RowCommitMeta:
		return []vaxis.Segment{
			{Text: row.Prefix, Style: d.commitLabelStyle()},
			{Text: row.Code, Style: d.commitMetaStyle()},
		}, true
	case diff.RowCommitTrailer:
		return []vaxis.Segment{
			{Text: row.Prefix, Style: d.commitTrailerLabelStyle()},
			{Text: row.Code, Style: d.commitTrailerValueStyle()},
		}, true
	default:
		return nil, false
	}
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
	trackTop := 0
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
		Row:     height - 2,
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
	point, ok := d.selectionPoint(mouse)
	if !ok {
		d.keys.Clear()
		d.mouseDrag = mouseDragState{
			Active: true,
			Mouse:  mouse,
			Row:    d.mouseDocumentRow(mouse),
		}
		if d.selection.Active {
			d.exitVisualMode()
			return CommandRedraw
		}
		return CommandNone
	}

	d.keys.Clear()
	d.clipboardText = ""
	d.setCursor(point)
	switch d.registerClick(point, time.Now()) {
	case 2:
		d.mouseDrag = mouseDragState{}
		return d.selectToken(point)
	case 3:
		d.mouseDrag = mouseDragState{}
		return d.selectRowAt(point)
	}

	d.exitVisualMode()
	d.mouseDrag = mouseDragState{
		Active:    true,
		Mouse:     mouse,
		Row:       point.Row,
		Anchor:    point,
		HasAnchor: true,
	}
	return CommandRedraw
}

func (d *diffViewer) registerClick(point selectionPoint, now time.Time) int {
	if d.clicks.Point == point && now.Sub(d.clicks.At) <= multiClickTimeout {
		d.clicks.Count++
	} else {
		d.clicks.Count = 1
	}
	if d.clicks.Count > 3 {
		d.clicks.Count = 1
	}
	d.clicks.Point = point
	d.clicks.At = now
	return d.clicks.Count
}

func (d *diffViewer) selectToken(point selectionPoint) Command {
	if point.Row < 0 || point.Row >= len(d.rows) {
		return CommandNone
	}

	start, end := tokenRangeAt(d.rows[point.Row].Text, point.Col)
	codeStart, codeEnd, ok := d.codeRange(d.rows[point.Row])
	if !ok {
		return CommandNone
	}
	start = maxInt(start, codeStart)
	end = minInt(end, codeEnd)
	d.selection = textSelection{
		Active: true,
		Anchor: selectionPoint{
			Row: point.Row,
			Col: start,
		},
		Cursor: selectionPoint{
			Row: point.Row,
			Col: maxInt(start, end-1),
		},
	}
	d.mode = modeVisual
	d.setCursor(selectionPoint{Row: point.Row, Col: maxInt(start, end-1)})
	return CommandRedraw
}

func (d *diffViewer) selectRowAt(point selectionPoint) Command {
	cmd := d.selectRow(point.Row)
	if cmd == CommandRedraw {
		d.setCursor(point)
	}
	return cmd
}

func (d *diffViewer) selectRow(row int) Command {
	if row < 0 || row >= len(d.rows) {
		return CommandNone
	}

	start, end, ok := d.codeRange(d.rows[row])
	if !ok {
		return CommandNone
	}
	d.selection = textSelection{
		Active: true,
		Anchor: selectionPoint{
			Row: row,
			Col: start,
		},
		Cursor: selectionPoint{
			Row: row,
			Col: maxInt(start, end-1),
		},
	}
	d.mode = modeVisualLine
	d.setCursor(selectionPoint{Row: row, Col: start})
	return CommandRedraw
}

func (d *diffViewer) extendSelection(mouse vaxis.Mouse) Command {
	point, ok := d.selectionPoint(mouse)

	if d.mouseDrag.Active {
		if !ok {
			return CommandNone
		}
		if !d.mouseDrag.Started {
			if !d.mouseDragExceeded(mouse) {
				return CommandNone
			}
			if !d.mouseDrag.HasAnchor {
				d.mouseDrag.Anchor = d.dragAnchor(point)
				d.mouseDrag.HasAnchor = true
			}
			d.mouseDrag.Started = true
			d.selection = textSelection{
				Active:   true,
				Dragging: true,
				Anchor:   d.mouseDrag.Anchor,
				Cursor:   point,
			}
			d.mode = modeVisual
		}
		d.selection.Cursor = point
		d.setCursor(point)
		return CommandRedraw
	}

	if !ok {
		return CommandNone
	}
	if !d.selection.Dragging {
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
	point, ok := d.selectionPoint(mouse)
	if !ok {
		return
	}
	d.selection.Cursor = point
	d.setCursor(point)
}

func (d *diffViewer) finishSelection(mouse vaxis.Mouse) Command {
	if d.mouseDrag.Active {
		started := d.mouseDrag.Started
		d.mouseDrag = mouseDragState{}
		if !started {
			return CommandNone
		}
	}
	if !d.selection.Dragging {
		return CommandNone
	}
	point, ok := d.selectionPoint(mouse)
	if ok {
		d.selection.Cursor = point
		d.setCursor(point)
	}
	d.selection.Dragging = false
	return CommandRedraw
}

func (d *diffViewer) selectionPoint(mouse vaxis.Mouse) (selectionPoint, bool) {
	row := d.mouseDocumentRow(mouse)
	if mouse.Row < 0 || mouse.Row >= d.visibleRowCapacity() || row < 0 || row >= len(d.rows) {
		return selectionPoint{}, false
	}

	docCol := d.documentColumn(d.rows[row], mouse.Col)
	start, end, ok := d.codeRange(d.rows[row])
	if !ok {
		return selectionPoint{}, false
	}
	if docCol < start {
		docCol = start
	}
	if docCol > end {
		docCol = end
	}
	return selectionPoint{Row: row, Col: docCol}, true
}

func (d *diffViewer) mouseDocumentRow(mouse vaxis.Mouse) int {
	return d.scroll + mouse.Row
}

func (d *diffViewer) dragAnchor(point selectionPoint) selectionPoint {
	start, end, ok := d.codeRange(d.rows[point.Row])
	if !ok {
		return point
	}
	switch {
	case point.Row > d.mouseDrag.Row:
		return selectionPoint{Row: point.Row, Col: start}
	case point.Row < d.mouseDrag.Row:
		return selectionPoint{Row: point.Row, Col: maxInt(start, end-1)}
	default:
		return point
	}
}

func (d *diffViewer) mouseDragExceeded(mouse vaxis.Mouse) bool {
	start := d.mouseDrag.Mouse
	return absInt(mouse.Col-start.Col) > 0 || absInt(mouse.Row-start.Row) > 0
}

func (d *diffViewer) documentColumn(row diff.Row, screenCol int) int {
	if screenCol < 0 {
		return 0
	}
	if row.Code == "" || row.Kind == diff.RowHunk {
		return screenCol
	}

	codeOffset := d.codeOffset(row)
	if screenCol < codeOffset {
		return screenCol
	}
	return codeOffset + d.xScroll + screenCol - codeOffset
}

func (d *diffViewer) codeRange(row diff.Row) (int, int, bool) {
	switch {
	case row.Kind == diff.RowHunk:
		return 0, 0, false
	case row.Gutter != "" || row.Marker != "":
		start := d.codeOffset(row)
		return start, start + textCellWidth(row.Code), true
	case row.Code != "":
		return 0, textCellWidth(row.Code), true
	default:
		return 0, 0, false
	}
}

func (d *diffViewer) paintSelection(win vaxis.Window, screenRow int, docRow int) {
	start, end, ok := d.selectionRangeForPaint(time.Now())
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
			win.SetCell(screenCol, screenRow, vaxis.Cell{
				Character: characterAtCell(row.Text, docCol),
				Style:     d.selectionCellStyle(row, docCol, style),
			})
		}
	}
}

func (d *diffViewer) selectionCellStyle(row diff.Row, docCol int, style vaxis.Style) vaxis.Style {
	if d.hasInlineReviewAt(row, docCol) {
		style.UnderlineColor = d.scheme.Yellow
		style.UnderlineStyle = vaxis.UnderlineCurly
	}
	return style
}

func (d *diffViewer) hasInlineReviewAt(row diff.Row, docCol int) bool {
	codeStart, _, ok := d.codeRange(row)
	if !ok {
		return false
	}
	start, end, ok := d.inlineReviewRange(row.Review)
	if !ok {
		return false
	}
	codeCol := docCol - codeStart
	return codeCol >= start && codeCol < end
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
	startCol, endCol, ok := d.codeRange(row)
	if !ok {
		return 0, 0, false
	}
	if d.mode == modeVisualLine {
		return startCol, endCol, true
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
	return selectionRange(d.selection)
}

func (d *diffViewer) selectionRangeForPaint(now time.Time) (selectionPoint, selectionPoint, bool) {
	if d.selection.Active {
		return selectionRange(d.selection)
	}
	if d.yankSelection.Active && !d.yankUntil.IsZero() && now.Before(d.yankUntil) {
		return selectionRange(d.yankSelection)
	}
	return selectionPoint{}, selectionPoint{}, false
}

func selectionRange(selection textSelection) (selectionPoint, selectionPoint, bool) {
	if !selection.Active {
		return selectionPoint{}, selectionPoint{}, false
	}

	start := selection.Anchor
	end := selection.Cursor
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
	if d.clipboardText != "" {
		return d.clipboardText
	}
	return d.selectionText()
}

func (d *diffViewer) prepareYank() bool {
	text := d.selectionText()
	if text == "" {
		return false
	}
	d.clipboardText = text
	d.yankSelection = d.selection
	d.exitVisualMode()
	return true
}

func (d *diffViewer) selectionText() string {
	start, end, ok := d.selectionRange()
	if !ok {
		return ""
	}

	var text strings.Builder
	wroteRow := false
	for rowIndex := start.Row; rowIndex <= end.Row && rowIndex < len(d.rows); rowIndex++ {
		rowStart, rowEnd, ok := d.codeRange(d.rows[rowIndex])
		if !ok {
			continue
		}
		if d.mode != modeVisualLine {
			if rowIndex == start.Row {
				rowStart = maxInt(rowStart, start.Col)
			}
			if rowIndex == end.Row {
				rowEnd = minInt(rowEnd, end.Col)
			}
		}
		var rowText string
		if rowStart < rowEnd {
			rowText = cellTextRange(d.rows[rowIndex].Text, rowStart, rowEnd)
			if textCellWidth(d.rows[rowIndex].Text) < rowEnd {
				rowText += " "
			}
		} else if d.selectedEmptyCell(rowIndex, rowStart, rowEnd) {
			rowText = " "
		}
		if rowText == "" {
			continue
		}
		if wroteRow {
			text.WriteByte('\n')
		}
		text.WriteString(rowText)
		wroteRow = true
	}
	return text.String()
}

func (d *diffViewer) selectedEmptyCell(rowIndex int, rowStart int, rowEnd int) bool {
	if rowStart != rowEnd {
		return false
	}
	start, end, ok := d.selectionRange()
	if !ok {
		return false
	}
	startCol, endCol, ok := d.selectionPaintRange(rowIndex, start, end)
	return ok && startCol == rowStart && endCol == rowStart+1
}

func (d *diffViewer) openReviewCommentEditor() bool {
	draft, ok := d.reviewDraftTarget()
	if !ok {
		return false
	}
	draftIndex := -1
	if index, ok := d.findReviewDraft(draft); ok {
		draftIndex = index
		draft = d.reviewDrafts[index]
	}
	d.editor = &commentEditor{
		draft:      draft,
		draftIndex: draftIndex,
		lines:      commentLines(draft.Body),
	}
	d.editor.row = len(d.editor.lines) - 1
	d.editor.col = utf8.RuneCountInString(d.editor.lines[d.editor.row])
	d.mode = modeInsert
	d.ensureCursorVisible()
	return true
}

func (d *diffViewer) handleCommentKey(key vaxis.Key) Command {
	switch {
	case key.Matches(vaxis.KeyEsc), key.MatchString("Esc"):
		d.submitReviewComment()
		return CommandRedraw
	case key.Matches(vaxis.KeyEnter):
		d.editor.insertLine()
		return CommandRedraw
	case key.Matches(vaxis.KeyBackspace), key.Matches('h', vaxis.ModCtrl):
		d.editor.backspace()
		return CommandRedraw
	case key.Matches(vaxis.KeyDelete):
		d.editor.deleteForward()
		return CommandRedraw
	case key.Matches(vaxis.KeyLeft):
		d.editor.moveCol(-1)
		return CommandRedraw
	case key.Matches(vaxis.KeyRight):
		d.editor.moveCol(1)
		return CommandRedraw
	case key.Matches(vaxis.KeyUp):
		d.editor.moveRow(-1)
		return CommandRedraw
	case key.Matches(vaxis.KeyDown):
		d.editor.moveRow(1)
		return CommandRedraw
	case key.Text != "" && key.Modifiers&(vaxis.ModCtrl|vaxis.ModAlt|vaxis.ModSuper) == 0:
		if d.editor.insertText(key.Text) {
			return CommandRedraw
		}
	}
	return CommandNone
}

func (d *diffViewer) enterCommandMode() {
	d.commandLine = ""
	d.statusMessage = ""
	d.mode = modeCommand
}

func (d *diffViewer) handleCommandKey(key vaxis.Key) Command {
	switch {
	case key.Matches(vaxis.KeyEsc), key.MatchString("Esc"):
		d.commandLine = ""
		d.mode = modeNormal
		return CommandRedraw
	case key.Matches(vaxis.KeyEnter):
		return d.executeCommand()
	case key.Matches(vaxis.KeyBackspace), key.Matches('h', vaxis.ModCtrl):
		if d.commandLine != "" {
			runes := []rune(d.commandLine)
			d.commandLine = string(runes[:len(runes)-1])
			return CommandRedraw
		}
	case key.Text != "" && key.Modifiers&(vaxis.ModCtrl|vaxis.ModAlt|vaxis.ModSuper) == 0:
		for _, r := range key.Text {
			if r >= ' ' {
				d.commandLine += string(r)
			}
		}
		return CommandRedraw
	}
	return CommandNone
}

func (d *diffViewer) enterSearchMode() {
	d.searchQuery = ""
	d.searchMatches = nil
	d.searchIndex = -1
	d.statusMessage = ""
	d.mode = modeSearch
}

func (d *diffViewer) clearSearch() bool {
	if d.searchQuery == "" && len(d.searchMatches) == 0 && d.searchIndex == -1 {
		return false
	}
	d.searchQuery = ""
	d.searchMatches = nil
	d.searchIndex = -1
	return true
}

func (d *diffViewer) handleSearchKey(key vaxis.Key) (Command, error) {
	switch {
	case key.Matches(vaxis.KeyEsc), key.MatchString("Esc"):
		d.mode = modeNormal
		d.clearSearch()
		return CommandRedraw, nil
	case key.Matches(vaxis.KeyEnter):
		d.updateSearchMatches()
		d.mode = modeNormal
		if len(d.searchMatches) == 0 {
			d.statusMessage = "Pattern not found"
			return CommandRedraw, nil
		}
		d.searchIndex = d.nextSearchIndexFromCursor(1)
		d.applySearchMatch()
		return CommandRedraw, nil
	case key.Matches(vaxis.KeyBackspace), key.Matches('h', vaxis.ModCtrl):
		if d.searchQuery != "" {
			runes := []rune(d.searchQuery)
			d.searchQuery = string(runes[:len(runes)-1])
			d.updateSearchMatches()
			return CommandRedraw, nil
		}
	case key.Text != "" && key.Modifiers&(vaxis.ModCtrl|vaxis.ModAlt|vaxis.ModSuper) == 0:
		for _, r := range key.Text {
			if r >= ' ' {
				d.searchQuery += string(r)
			}
		}
		d.updateSearchMatches()
		return CommandRedraw, nil
	}
	return CommandNone, nil
}

func (d *diffViewer) executeCommand() Command {
	command := strings.TrimSpace(d.commandLine)
	d.commandLine = ""
	return d.executeCommandString(command)
}

func (d *diffViewer) executeCommandString(command string) Command {
	if command == "" {
		d.mode = modeNormal
		return CommandRedraw
	}

	for len(command) > 0 {
		switch {
		case strings.HasPrefix(command, "q!"):
			return CommandQuit
		case strings.HasPrefix(command, "q"):
			if d.reviewDirty {
				d.mode = modeNormal
				d.statusMessage = "Unsaved comments. Use :w to save or :q! to quit."
				return CommandRedraw
			}
			return CommandQuit
		case strings.HasPrefix(command, "w"):
			d.writeReviewCommand()
			command = command[1:]
		default:
			d.mode = modeNormal
			return CommandRedraw
		}
	}

	d.mode = modeNormal
	return CommandRedraw
}

func (d *diffViewer) writeReviewCommand() Command {
	if d.editor != nil {
		d.submitReviewComment()
	}
	if err := d.saveReviewComments(); err != nil {
		d.mode = modeNormal
		d.statusMessage = fmt.Sprintf("Could not save comments: %v", err)
		return CommandRedraw
	}
	d.reviewDirty = false
	d.mode = modeNormal
	d.statusMessage = "Comments saved."
	return CommandRedraw
}

func (d *diffViewer) saveReviewComments() error {
	if d.reviewFile == "" {
		return nil
	}
	return review.SaveFile(d.reviewFile, review.CommentFile{
		Version:  1,
		Comments: d.reviewDrafts,
	})
}

func (d *diffViewer) submitReviewComment() bool {
	if d.editor == nil {
		return false
	}
	body := d.editor.body()
	draftIndex := d.editor.draftIndex
	if strings.TrimSpace(body) == "" {
		if draftIndex >= 0 && draftIndex < len(d.reviewDrafts) {
			d.reviewDrafts = append(d.reviewDrafts[:draftIndex], d.reviewDrafts[draftIndex+1:]...)
			d.reviewDirty = true
		}
		d.editor = nil
		d.mode = modeNormal
		return true
	}

	draft := d.editor.draft
	draft.Body = body
	if draftIndex >= 0 && draftIndex < len(d.reviewDrafts) {
		if d.reviewDrafts[draftIndex] != draft {
			d.reviewDirty = true
		}
		d.reviewDrafts[draftIndex] = draft
	} else {
		d.reviewDrafts = append(d.reviewDrafts, draft)
		d.reviewDirty = true
	}
	d.editor = nil
	d.exitVisualMode()
	d.mode = modeNormal
	return true
}

func (d *diffViewer) findReviewDraft(target review.CommentDraft) (int, bool) {
	for index, draft := range d.reviewDrafts {
		if reviewDraftMatchesTarget(draft, target) {
			return index, true
		}
	}
	return 0, false
}

func (d *diffViewer) updateSearchMatches() {
	if d.searchQuery == "" {
		d.searchMatches = nil
		d.searchIndex = -1
		return
	}

	query := strings.ToLower(d.searchQuery)
	var matches []searchMatch
	for rowIndex, row := range d.rows {
		searchText, offset := d.searchableText(row)
		text := strings.ToLower(searchText)
		for start := 0; ; {
			index := strings.Index(text[start:], query)
			if index < 0 {
				break
			}
			matchStart := start + index
			matchEnd := matchStart + len(query)
			matches = append(matches, searchMatch{
				Row:   rowIndex,
				Start: offset + textCellWidth(searchText[:matchStart]),
				End:   offset + textCellWidth(searchText[:matchEnd]),
			})
			start = matchEnd
		}
	}
	d.searchMatches = matches
	d.searchIndex = -1
}

func (d *diffViewer) searchableText(row diff.Row) (string, int) {
	if row.Code != "" && (row.Gutter != "" || row.Marker != "") {
		return row.Code, d.codeOffset(row)
	}
	return row.Text, 0
}

func (d *diffViewer) moveSearchMatch(delta int) bool {
	if len(d.searchMatches) == 0 {
		return false
	}
	if d.searchIndex < 0 || d.searchIndex >= len(d.searchMatches) {
		d.searchIndex = d.nextSearchIndexFromCursor(delta)
	} else {
		d.searchIndex = (d.searchIndex + delta + len(d.searchMatches)) % len(d.searchMatches)
	}
	d.applySearchMatch()
	return true
}

func (d *diffViewer) nextSearchIndexFromCursor(direction int) int {
	if len(d.searchMatches) == 0 {
		return -1
	}
	if direction < 0 {
		for index := len(d.searchMatches) - 1; index >= 0; index-- {
			if selectionPointLess(selectionPoint{Row: d.searchMatches[index].Row, Col: d.searchMatches[index].Start}, d.cursor) {
				return index
			}
		}
		return len(d.searchMatches) - 1
	}
	for index, match := range d.searchMatches {
		point := selectionPoint{Row: match.Row, Col: match.Start}
		if selectionPointLess(d.cursor, point) || d.cursor == point {
			return index
		}
	}
	return 0
}

func (d *diffViewer) applySearchMatch() {
	if d.searchIndex < 0 || d.searchIndex >= len(d.searchMatches) {
		return
	}
	match := d.searchMatches[d.searchIndex]
	d.setCursor(selectionPoint{Row: match.Row, Col: match.Start})
	d.statusMessage = ""
}

func (d *diffViewer) searchSegments(rowIndex int, row diff.Row, segments []vaxis.Segment) []vaxis.Segment {
	if len(d.searchMatches) == 0 || rowIndex < 0 {
		return segments
	}
	style := vaxis.Style{
		Foreground: d.scheme.Background,
		Background: d.scheme.Yellow,
	}
	for _, match := range d.searchMatches {
		if match.Row != rowIndex {
			continue
		}
		start := match.Start
		end := match.End
		if row.Gutter != "" || row.Marker != "" {
			offset := d.codeOffset(row)
			start -= offset
			end -= offset
		}
		segments = styleSegmentsRangeFull(segments, start, end, style)
	}
	return segments
}

func reviewDraftMatchesTarget(draft review.CommentDraft, target review.CommentDraft) bool {
	if draft.Path != target.Path ||
		draft.Line != target.Line ||
		draft.Side != target.Side ||
		draft.StartLine != target.StartLine ||
		draft.StartSide != target.StartSide {
		return false
	}
	return optionalIntEqual(draft.StartColumn, target.StartColumn) &&
		optionalIntEqual(draft.EndColumn, target.EndColumn)
}

func optionalIntEqual(a *int, b *int) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func commentLines(body string) []string {
	if body == "" {
		return []string{""}
	}
	lines := strings.Split(body, "\n")
	if len(lines) > commentEditorRows {
		lines = lines[:commentEditorRows]
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func (e *commentEditor) body() string {
	return strings.Join(e.lines, "\n")
}

func (e *commentEditor) insertText(text string) bool {
	changed := false
	for _, r := range text {
		switch {
		case r == '\r':
			continue
		case r == '\n':
			if e.insertLine() {
				changed = true
			}
		case r == '\t' || r >= ' ':
			e.insertRune(r)
			changed = true
		}
	}
	return changed
}

func (e *commentEditor) insertRune(r rune) {
	line := e.lines[e.row]
	runes := []rune(line)
	if e.col > len(runes) {
		e.col = len(runes)
	}
	runes = append(runes, 0)
	copy(runes[e.col+1:], runes[e.col:])
	runes[e.col] = r
	e.lines[e.row] = string(runes)
	e.col++
}

func (e *commentEditor) insertLine() bool {
	if len(e.lines) >= commentEditorRows {
		return false
	}
	line := e.lines[e.row]
	runes := []rune(line)
	if e.col > len(runes) {
		e.col = len(runes)
	}
	left := string(runes[:e.col])
	right := string(runes[e.col:])
	e.lines[e.row] = left
	e.lines = append(e.lines, "")
	copy(e.lines[e.row+2:], e.lines[e.row+1:])
	e.lines[e.row+1] = right
	e.row++
	e.col = 0
	return true
}

func (e *commentEditor) backspace() {
	if e.col > 0 {
		line := []rune(e.lines[e.row])
		e.lines[e.row] = string(append(line[:e.col-1], line[e.col:]...))
		e.col--
		return
	}
	if e.row == 0 {
		return
	}

	prev := e.lines[e.row-1]
	e.col = utf8.RuneCountInString(prev)
	e.lines[e.row-1] = prev + e.lines[e.row]
	e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
	e.row--
}

func (e *commentEditor) deleteForward() {
	line := []rune(e.lines[e.row])
	if e.col < len(line) {
		e.lines[e.row] = string(append(line[:e.col], line[e.col+1:]...))
		return
	}
	if e.row+1 >= len(e.lines) {
		return
	}
	e.lines[e.row] += e.lines[e.row+1]
	e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
}

func (e *commentEditor) moveCol(delta int) {
	e.col += delta
	if e.col < 0 {
		if e.row == 0 {
			e.col = 0
			return
		}
		e.row--
		e.col = utf8.RuneCountInString(e.lines[e.row])
		return
	}

	lineLen := utf8.RuneCountInString(e.lines[e.row])
	if e.col <= lineLen {
		return
	}
	if e.row+1 >= len(e.lines) {
		e.col = lineLen
		return
	}
	e.row++
	e.col = 0
}

func (e *commentEditor) moveRow(delta int) {
	e.row += delta
	if e.row < 0 {
		e.row = 0
	}
	if e.row >= len(e.lines) {
		e.row = len(e.lines) - 1
	}
	lineLen := utf8.RuneCountInString(e.lines[e.row])
	if e.col > lineLen {
		e.col = lineLen
	}
}

func (d *diffViewer) reviewDraftTarget() (review.CommentDraft, bool) {
	if d.selection.Active {
		return d.reviewDraftForSelection()
	}
	if d.cursor.Row < 0 || d.cursor.Row >= len(d.rows) {
		return review.CommentDraft{}, false
	}
	anchor := d.rows[d.cursor.Row].Review
	if !reviewAnchorValid(anchor) {
		return review.CommentDraft{}, false
	}
	return review.CommentDraft{
		Path:             anchor.Path,
		Line:             anchor.Line,
		Side:             anchor.Side,
		CommitID:         anchor.CommitID,
		OriginalCommitID: anchor.OriginalCommitID,
	}, true
}

func (d *diffViewer) reviewDraftForSelection() (review.CommentDraft, bool) {
	start, end, ok := d.selectionRange()
	if !ok {
		return review.CommentDraft{}, false
	}

	startAnchor, ok := d.firstReviewAnchor(start.Row, end.Row)
	if !ok {
		return review.CommentDraft{}, false
	}
	endAnchor, ok := d.lastReviewAnchor(start.Row, end.Row)
	if !ok {
		return review.CommentDraft{}, false
	}

	draft := review.CommentDraft{
		Path:             startAnchor.Path,
		Line:             endAnchor.Line,
		Side:             endAnchor.Side,
		CommitID:         endAnchor.CommitID,
		OriginalCommitID: endAnchor.OriginalCommitID,
	}
	if startAnchor.Path != endAnchor.Path {
		return review.CommentDraft{}, false
	}
	if startAnchor.Line != endAnchor.Line || startAnchor.Side != endAnchor.Side {
		draft.StartLine = startAnchor.Line
		draft.StartSide = startAnchor.Side
	}
	if d.mode == modeVisual && start.Row == end.Row && startAnchor.Line == endAnchor.Line && startAnchor.Side == endAnchor.Side {
		startColumn, endColumn, ok := d.reviewColumnsForSelection(start.Row, start.Col, end.Col)
		if ok {
			draft.StartColumn = &startColumn
			draft.EndColumn = &endColumn
		}
	}
	return draft, true
}

func (d *diffViewer) reviewColumnsForSelection(rowIndex int, startCol int, endCol int) (int, int, bool) {
	if rowIndex < 0 || rowIndex >= len(d.rows) {
		return 0, 0, false
	}
	codeStart, codeEnd, ok := d.codeRange(d.rows[rowIndex])
	if !ok {
		return 0, 0, false
	}
	startCol = maxInt(startCol, codeStart)
	endCol = minInt(endCol, codeEnd)
	if startCol >= endCol {
		return 0, 0, false
	}
	return startCol - codeStart + 1, endCol - codeStart, true
}

func (d *diffViewer) firstReviewAnchor(start int, end int) (review.Anchor, bool) {
	if start < 0 {
		start = 0
	}
	if end >= len(d.rows) {
		end = len(d.rows) - 1
	}
	for row := start; row <= end; row++ {
		if reviewAnchorValid(d.rows[row].Review) {
			return d.rows[row].Review, true
		}
	}
	return review.Anchor{}, false
}

func (d *diffViewer) lastReviewAnchor(start int, end int) (review.Anchor, bool) {
	if start < 0 {
		start = 0
	}
	if end >= len(d.rows) {
		end = len(d.rows) - 1
	}
	for row := end; row >= start; row-- {
		if reviewAnchorValid(d.rows[row].Review) {
			return d.rows[row].Review, true
		}
	}
	return review.Anchor{}, false
}

func (d *diffViewer) hasReviewDraft(anchor review.Anchor) bool {
	if !reviewAnchorValid(anchor) {
		return false
	}
	for _, draft := range d.reviewDrafts {
		if draft.Path == anchor.Path && draft.Line == anchor.Line && draft.Side == anchor.Side {
			return true
		}
		if draft.Path == anchor.Path && reviewDraftContains(draft, anchor) {
			return true
		}
	}
	return false
}

func reviewDraftContains(draft review.CommentDraft, anchor review.Anchor) bool {
	if draft.StartLine == 0 {
		return draft.Line == anchor.Line && draft.Side == anchor.Side
	}
	if draft.StartSide != anchor.Side || draft.Side != anchor.Side {
		return draft.Line == anchor.Line && draft.Side == anchor.Side ||
			draft.StartLine == anchor.Line && draft.StartSide == anchor.Side
	}
	start := minInt(draft.StartLine, draft.Line)
	end := maxInt(draft.StartLine, draft.Line)
	return anchor.Side == draft.Side && anchor.Line >= start && anchor.Line <= end
}

func reviewAnchorValid(anchor review.Anchor) bool {
	return anchor.Path != "" && anchor.Line > 0 && anchor.Side != ""
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
	if start, end, ok := d.codeRange(d.rows[row]); ok {
		if col < start {
			return start
		}
		if col > end {
			return end
		}
		return col
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
	d.updateVisualSelection()
}

func (d *diffViewer) moveCursorCols(delta int) {
	d.prepareCursorForMovement()
	d.cursor.Col += delta
	d.clampCursor()
	d.cursorGoal = d.cursor.Col
	d.ensureCursorVisible()
	d.updateVisualSelection()
}

func (d *diffViewer) cursorTop() {
	d.cursor.Row = 0
	d.clampCursor()
	d.cursorGoal = d.cursor.Col
	d.ensureCursorVisible()
	d.updateVisualSelection()
}

func (d *diffViewer) cursorBottom() {
	d.cursor.Row = len(d.rows) - 1
	d.clampCursor()
	d.cursorGoal = d.cursor.Col
	d.ensureCursorVisible()
	d.updateVisualSelection()
}

func (d *diffViewer) enterVisualMode() {
	if len(d.rows) == 0 {
		return
	}
	start, end, ok := d.codeRange(d.rows[d.cursor.Row])
	if !ok {
		return
	}
	col := d.cursor.Col
	if col < start {
		col = start
	}
	if col > end {
		col = end
	}
	d.cursor.Col = col
	d.cursorGoal = col
	d.mode = modeVisual
	d.clipboardText = ""
	d.selection = textSelection{
		Active: true,
		Anchor: selectionPoint{
			Row: d.cursor.Row,
			Col: col,
		},
		Cursor: selectionPoint{
			Row: d.cursor.Row,
			Col: col,
		},
	}
}

func (d *diffViewer) enterVisualLineMode() {
	if len(d.rows) == 0 {
		return
	}
	start, end, ok := d.codeRange(d.rows[d.cursor.Row])
	if !ok {
		return
	}
	d.mode = modeVisualLine
	d.clipboardText = ""
	d.selection = textSelection{
		Active: true,
		Anchor: selectionPoint{
			Row: d.cursor.Row,
			Col: start,
		},
		Cursor: selectionPoint{
			Row: d.cursor.Row,
			Col: maxInt(start, end-1),
		},
	}
}

func (d *diffViewer) exitVisualMode() {
	d.mode = modeNormal
	d.selection = textSelection{}
}

func (d *diffViewer) updateVisualSelection() {
	switch d.mode {
	case modeVisual:
		if d.selection.Active {
			d.clampCursorToCode()
			d.selection.Cursor = d.cursor
		}
	case modeVisualLine:
		if !d.selection.Active {
			return
		}
		row := d.cursor.Row
		start, end, ok := d.codeRange(d.rows[row])
		if !ok {
			d.selection.Cursor = selectionPoint{Row: row, Col: 0}
			return
		}
		if row < d.selection.Anchor.Row {
			d.selection.Cursor = selectionPoint{Row: row, Col: start}
		} else {
			d.selection.Cursor = selectionPoint{Row: row, Col: maxInt(start, end-1)}
		}
	}
}

func (d *diffViewer) clampCursorToCode() {
	if d.cursor.Row < 0 || d.cursor.Row >= len(d.rows) {
		return
	}
	start, end, ok := d.codeRange(d.rows[d.cursor.Row])
	if !ok {
		return
	}
	if d.cursor.Col < start {
		d.cursor.Col = start
	}
	if d.cursor.Col > end {
		d.cursor.Col = end
	}
	d.cursorGoal = d.cursor.Col
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
		d.ensureCommentEditorVisible(visible)
		d.clampScroll()
	}

	d.ensureCursorColumnVisible()
}

func (d *diffViewer) ensureCommentEditorVisible(visible int) {
	if d.editor == nil || visible <= 0 {
		return
	}

	minScroll := d.cursor.Row + commentEditorBoxHeight + 1 - visible
	if minScroll > d.scroll {
		d.scroll = minScroll
	}
	if d.scroll > d.cursor.Row {
		d.scroll = d.cursor.Row
	}
	if d.scroll < 0 {
		d.scroll = 0
	}
}

func (d *diffViewer) ensureCursorColumnVisible() {
	if d.cursor.Row < 0 || d.cursor.Row >= len(d.rows) {
		return
	}

	row := d.rows[d.cursor.Row]
	if row.Code == "" || row.Kind == diff.RowHunk {
		return
	}

	codeOffset := d.codeOffset(row)
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
		if d.editor != nil {
			editorScroll := d.cursor.Row + commentEditorBoxHeight + 1 - visible
			if editorScroll > maxScroll {
				maxScroll = editorScroll
			}
		}
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
	case diff.RowCommitHeader:
		return d.commitHashStyle()
	case diff.RowCommitMeta:
		return d.commitMetaStyle()
	case diff.RowCommitMessage:
		return vaxis.Style{
			Foreground: d.scheme.Foreground,
			Background: d.scheme.Background,
		}
	case diff.RowCommitTrailer:
		return d.commitTrailerValueStyle()
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

func (d *diffViewer) commitHashStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Yellow,
		Background: d.scheme.Background,
		Attribute:  vaxis.AttrBold,
	}
}

func (d *diffViewer) commitLabelStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Muted,
		Background: d.scheme.Background,
	}
}

func (d *diffViewer) commitMetaStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Base.Cyan,
		Background: d.scheme.Background,
	}
}

func (d *diffViewer) commitTrailerLabelStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Blue,
		Background: d.scheme.Background,
		Attribute:  vaxis.AttrBold,
	}
}

func (d *diffViewer) commitTrailerValueStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Dim,
		Background: d.scheme.Background,
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
	text := row.Gutter + row.Marker
	style := d.gutterStyle(row.Kind)
	if !d.hasReviewDraft(row.Review) || !strings.HasSuffix(text, " ") {
		return []vaxis.Segment{{Text: text, Style: style}}
	}

	markerStyle := style
	markerStyle.Foreground = d.scheme.Yellow
	return []vaxis.Segment{
		{Text: text[:len(text)-1], Style: style},
		{Text: "▐", Style: markerStyle},
	}
}

func (d *diffViewer) reviewSegments(row diff.Row, segments []vaxis.Segment) []vaxis.Segment {
	start, end, ok := d.inlineReviewRange(row.Review)
	if !ok {
		return segments
	}
	style := vaxis.Style{
		UnderlineColor: d.scheme.Yellow,
		UnderlineStyle: vaxis.UnderlineCurly,
	}
	return styleSegmentsRange(segments, start, end, style)
}

func (d *diffViewer) inlineReviewRange(anchor review.Anchor) (int, int, bool) {
	if !reviewAnchorValid(anchor) {
		return 0, 0, false
	}
	for _, draft := range d.reviewDrafts {
		if draft.Path != anchor.Path || draft.Line != anchor.Line || draft.Side != anchor.Side {
			continue
		}
		if draft.StartColumn == nil || draft.EndColumn == nil {
			continue
		}
		start := *draft.StartColumn - 1
		end := *draft.EndColumn
		if start < 0 {
			start = 0
		}
		if end <= start {
			continue
		}
		return start, end, true
	}
	return 0, 0, false
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

func segmentStyleAt(segments []vaxis.Segment, target int) (vaxis.Style, bool) {
	if target < 0 {
		return vaxis.Style{}, false
	}

	col := 0
	for _, segment := range segments {
		next := col + textCellWidth(segment.Text)
		if target >= col && target < next {
			return segment.Style, true
		}
		col = next
	}
	return vaxis.Style{}, false
}

func styleSegmentsRange(segments []vaxis.Segment, start int, end int, style vaxis.Style) []vaxis.Segment {
	if start >= end {
		return segments
	}

	var styled []vaxis.Segment
	col := 0
	for _, segment := range segments {
		for _, char := range vaxis.Characters(segment.Text) {
			next := col + char.Width
			charStyle := segment.Style
			if next > start && col < end {
				charStyle.UnderlineColor = style.UnderlineColor
				charStyle.UnderlineStyle = style.UnderlineStyle
			}
			styled = appendSegment(styled, vaxis.Segment{
				Text:  char.Grapheme,
				Style: charStyle,
			})
			col = next
		}
	}
	return styled
}

func styleSegmentsRangeFull(segments []vaxis.Segment, start int, end int, style vaxis.Style) []vaxis.Segment {
	if start >= end {
		return segments
	}

	var styled []vaxis.Segment
	col := 0
	for _, segment := range segments {
		for _, char := range vaxis.Characters(segment.Text) {
			next := col + char.Width
			charStyle := segment.Style
			if next > start && col < end {
				if style.Foreground != vaxis.ColorDefault {
					charStyle.Foreground = style.Foreground
				}
				if style.Background != vaxis.ColorDefault {
					charStyle.Background = style.Background
				}
				if style.UnderlineColor != vaxis.ColorDefault {
					charStyle.UnderlineColor = style.UnderlineColor
				}
				if style.UnderlineStyle != vaxis.UnderlineOff {
					charStyle.UnderlineStyle = style.UnderlineStyle
				}
				charStyle.Attribute |= style.Attribute
			}
			styled = appendSegment(styled, vaxis.Segment{
				Text:  char.Grapheme,
				Style: charStyle,
			})
			col = next
		}
	}
	return styled
}

func appendSegment(segments []vaxis.Segment, segment vaxis.Segment) []vaxis.Segment {
	if segment.Text == "" {
		return segments
	}
	last := len(segments) - 1
	if last >= 0 && segments[last].Style == segment.Style {
		segments[last].Text += segment.Text
		return segments
	}
	return append(segments, segment)
}

func textCellWidth(text string) int {
	width := 0
	for _, char := range vaxis.Characters(text) {
		width += char.Width
	}
	return width
}

func characterAtCell(text string, target int) vaxis.Character {
	if target < 0 {
		target = 0
	}

	col := 0
	for _, char := range vaxis.Characters(text) {
		next := col + char.Width
		if target >= col && target < next {
			if target == col && char.Width == 1 {
				return char
			}
			break
		}
		col = next
	}
	return vaxis.Character{
		Grapheme: " ",
		Width:    1,
	}
}

func runeAtIndex(text string, index int) rune {
	if index < 0 {
		return ' '
	}
	for _, r := range text {
		if index == 0 {
			return r
		}
		index--
	}
	return ' '
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
	it := uucode.NewGraphemeIterator(text)
	for g, ok := it.Next(); ok; g, ok = it.Next() {
		cluster := text[g.Start:g.End]
		next := col + graphemeCellWidth(cluster)
		if next > start && col < end {
			out.WriteString(cluster)
		}
		col = next
		if col >= end {
			break
		}
	}
	return out.String()
}

func graphemeCellWidth(grapheme string) int {
	if grapheme == "\t" {
		return 8
	}
	return uucode.StringWidth(grapheme)
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

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
