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
const commentEditorMaxRows = 8
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
	layoutMode    diffLayoutMode
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
	textObject    textObjectState
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
	scroll     int
}

type commentEditorLayout struct {
	x             int
	y             int
	boxWidth      int
	boxHeight     int
	inputWidth    int
	visibleRows   int
	showScrollbar bool
	wrapped       []commentDisplayLine
}

type commentDisplayLine struct {
	line  int
	start int
	end   int
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

type diffLayoutMode int

const (
	layoutStacked diffLayoutMode = iota
	layoutSideBySide
)

type sideBySideRow struct {
	Full  int
	Left  int
	Right int
}

type textObjectKind int

const (
	textObjectInner textObjectKind = iota
	textObjectAround
)

type textObjectState struct {
	active bool
	kind   textObjectKind
	at     time.Time
}

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
	d.clearExpiredTextObject(time.Now())
	if d.textObject.active {
		return d.handleTextObjectKey(key)
	}

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
	case key.Matches('s'):
		d.keys.Clear()
		d.toggleLayoutMode()
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
	case key.Matches('a'):
		d.keys.Clear()
		if !d.inVisualMode() {
			return CommandNone, nil
		}
		d.startTextObject(textObjectAround)
		return CommandNone, nil
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
	case key.Matches('J'):
		d.keys.Clear()
		if !d.jumpCommit(1) {
			return CommandNone, nil
		}
		return CommandRedraw, nil
	case key.Matches('K'):
		d.keys.Clear()
		if !d.jumpCommit(-1) {
			return CommandNone, nil
		}
		return CommandRedraw, nil
	case key.Matches('D'):
		d.keys.Clear()
		if !d.deleteReviewDraftAtTarget() {
			return CommandNone, nil
		}
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
	case key.Matches('0'):
		d.keys.Clear()
		d.cursorLineStart()
		return CommandRedraw, nil
	case key.Matches('$'):
		d.keys.Clear()
		d.cursorLineEnd()
		return CommandRedraw, nil
	case key.Matches('I'):
		d.keys.Clear()
		if d.editor != nil {
			d.mode = modeInsert
			return CommandRedraw, nil
		}
		if !d.openReviewCommentEditor() {
			return CommandNone, nil
		}
		return CommandRedraw, nil
	case key.Matches('i'):
		d.keys.Clear()
		if d.inVisualMode() {
			d.startTextObject(textObjectInner)
			return CommandNone, nil
		}
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

func (d *diffViewer) startTextObject(kind textObjectKind) {
	d.textObject = textObjectState{
		active: true,
		kind:   kind,
		at:     time.Now(),
	}
}

func (d *diffViewer) inVisualMode() bool {
	return d.mode == modeVisual || d.mode == modeVisualLine
}

func (d *diffViewer) clearExpiredTextObject(now time.Time) {
	if d.textObject.active && now.Sub(d.textObject.at) > pendingKeyTimeout {
		d.textObject = textObjectState{}
	}
}

func (d *diffViewer) handleTextObjectKey(key vaxis.Key) (Command, error) {
	state := d.textObject
	d.textObject = textObjectState{}

	if key.Matches(vaxis.KeyEsc) || key.MatchString("Esc") {
		return CommandNone, nil
	}

	if key.Text == "" {
		return CommandNone, nil
	}
	r := key.Keycode
	if r == 0 {
		r, _ = utf8.DecodeRuneInString(key.Text)
	}
	if r == utf8.RuneError {
		return CommandNone, nil
	}
	if !d.selectTextObject(state.kind, r) {
		return CommandNone, nil
	}
	return CommandRedraw, nil
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

	if d.layoutMode == layoutSideBySide {
		d.paintSideBySide(win)
	} else {
		for row, diffRow := range d.visibleRows() {
			docRow := d.scroll + row
			d.printRow(win, row, docRow, diffRow, d.codeSegments[docRow], docRow == d.cursor.Row)
			d.paintSelection(win, row, docRow)
		}
	}
	d.paintStickyFileHeader(win)
	d.paintCursor(win)
	d.paintScrollbar(win)
	d.paintHorizontalScrollbar(win)
	d.paintCommentEditor(win)
	d.paintStatusBar(win)
}

func (d *diffViewer) paintStickyFileHeader(win vaxis.Window) {
	if d.layoutMode == layoutSideBySide {
		return
	}
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
	if d.mode == modeCommand || d.mode == modeInsert {
		return
	}

	col, row, ok := d.cursorScreenPosition(win)
	if !ok {
		return
	}
	if win.Vx != nil {
		win.ShowCursor(col, row, vaxis.CursorBlock)
	}
}

func (d *diffViewer) cursorScreenPosition(win vaxis.Window) (int, int, bool) {
	width, height := win.Size()
	return d.cursorScreenPositionForSize(width, height)
}

func (d *diffViewer) cursorScreenPositionForSize(width int, height int) (int, int, bool) {
	if d.layoutMode == layoutSideBySide {
		return d.sideBySideCursorScreenPosition(width, height)
	}
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

func (d *diffViewer) codeOffset(row diff.Row) int {
	return textCellWidth(row.Gutter + row.Marker)
}

func (d *diffViewer) codeSegmentsForRow(row int) []vaxis.Segment {
	if row < 0 || row >= len(d.codeSegments) {
		return nil
	}
	return d.codeSegments[row]
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
			Style: d.statusTextStyle(),
		})
		d.paintCommandCursor(win)
		return
	}
	if d.mode == modeSearch {
		printSegmentsAt(win, 0, row, vaxis.Segment{
			Text:  "/" + d.searchQuery,
			Style: d.statusTextStyle(),
		})
		d.paintSearchCursor(win)
		return
	}
	leftSegments := d.statusLeftSegments()
	separatorBackground := d.statusBackground()
	if len(leftSegments) > 0 {
		separatorBackground = leftSegments[0].Style.Background
	}
	modeSegments := d.statusModeSegments(separatorBackground)
	modeWidth := segmentsWidth(modeSegments)
	printSegmentsAt(win, 0, row, modeSegments...)
	if d.statusMessage != "" {
		printSegmentsAt(win, modeWidth, row, vaxis.Segment{
			Text:  d.statusMessage,
			Style: d.statusDimStyle(),
		})
		return
	}

	rightSegments := d.statusRightSegments()
	rightWidth := segmentsWidth(rightSegments)
	rightCol := width - rightWidth - 1
	if rightCol < modeWidth {
		rightSegments = nil
		rightWidth = 0
		rightCol = width
	}

	leftWidth := rightCol - modeWidth - 1
	if leftWidth < 0 {
		leftWidth = 0
	}
	printSegmentsClipped(win, modeWidth, row, leftWidth, leftSegments...)
	if len(rightSegments) > 0 {
		printSegmentsAt(win, width-rightWidth-1, row, rightSegments...)
	}
}

func (d *diffViewer) statusModeSegments(separatorBackground vaxis.Color) []vaxis.Segment {
	return []vaxis.Segment{
		vaxis.Segment{
			Text:  " " + d.modeLabel() + " ",
			Style: d.statusStyle(),
		},
		vaxis.Segment{
			Text:  "",
			Style: d.statusSeparatorStyle(separatorBackground),
		},
	}
}

type statusStats struct {
	Adds    int
	Deletes int
}

type statusContext struct {
	CommitIndex int
	Commits     int
	Commit      string
	FileIndex   int
	Files       int
	File        string
	FileStats   statusStats
	TotalStats  statusStats
}

func (d *diffViewer) statusLeftSegments() []vaxis.Segment {
	context := d.currentStatusContext()
	sections := make([]statusSection, 0, 2)

	if context.Commits > 0 && context.CommitIndex > 0 {
		commit := context.Commit
		if len(commit) > 12 {
			commit = commit[:12]
		}
		if commit != "" {
			if context.Commits > 1 {
				commit = fmt.Sprintf("%d/%d %s", context.CommitIndex, context.Commits, commit)
			}
			sections = append(sections, statusSection{
				Text:       commit,
				Foreground: d.scheme.Base.Blue,
				Background: d.statusCommitBackground(),
			})
		}
	}

	if context.Files > 0 && context.File != "" {
		file := context.File
		if context.Files > 1 {
			file = fmt.Sprintf("%d/%d %s", context.FileIndex, context.Files, file)
		}
		sections = append(sections, statusSection{
			Text:       file,
			Foreground: d.scheme.Foreground,
			Background: d.statusBackground(),
			Separator:  "",
			PathBase:   true,
		})
	}

	segments := d.statusSectionSegments(sections)
	if context.Files > 0 && context.File != "" {
		segments = append(segments, d.statusStatsSegments(context.FileStats)...)
	}
	if d.layoutMode == layoutSideBySide {
		segments = append([]vaxis.Segment{{Text: " SIDE-BY-SIDE ", Style: d.statusDimStyle()}}, segments...)
	}
	return segments
}

func (d *diffViewer) statusRightSegments() []vaxis.Segment {
	context := d.currentStatusContext()
	if context.Commits == 0 && context.Files == 0 {
		return nil
	}
	return append([]vaxis.Segment{{
		Text:  fmt.Sprintf("%s / %s ", countLabel(context.Commits, "commit"), countLabel(context.Files, "file")),
		Style: d.statusDimStyle(),
	}}, d.statusStatsSegments(context.TotalStats)...)
}

type statusSection struct {
	Text       string
	Foreground vaxis.Color
	Background vaxis.Color
	Separator  string
	PathBase   bool
}

func (d *diffViewer) statusSectionSegments(sections []statusSection) []vaxis.Segment {
	segments := make([]vaxis.Segment, 0, len(sections)*2)
	for index, section := range sections {
		if section.Text == "" {
			continue
		}
		nextBackground := d.statusBackground()
		if index+1 < len(sections) {
			nextBackground = sections[index+1].Background
		}
		text := " " + section.Text + " "
		separator := section.Separator
		if separator == "" {
			separator = ""
		}
		separatorStyle := vaxis.Style{
			Foreground: section.Background,
			Background: nextBackground,
		}
		if separator == "" {
			separatorStyle = vaxis.Style{
				Foreground: section.Foreground,
				Background: section.Background,
			}
		}
		segments = append(segments, d.statusSectionTextSegments(section, text)...)
		segments = append(segments, vaxis.Segment{
			Text:  separator,
			Style: separatorStyle,
		})
	}
	return segments
}

func (d *diffViewer) statusSectionTextSegments(section statusSection, text string) []vaxis.Segment {
	style := vaxis.Style{
		Foreground: section.Foreground,
		Background: section.Background,
		Attribute:  vaxis.AttrBold,
	}
	if !section.PathBase {
		return []vaxis.Segment{{Text: text, Style: style}}
	}

	regularStyle := style
	regularStyle.Attribute = 0
	prefix, base := splitStatusPathBase(strings.TrimSpace(text))
	segments := make([]vaxis.Segment, 0, 3)
	if prefix != "" {
		segments = append(segments, vaxis.Segment{Text: " " + prefix, Style: regularStyle})
	} else {
		segments = append(segments, vaxis.Segment{Text: " ", Style: regularStyle})
	}
	segments = append(segments, vaxis.Segment{Text: base, Style: style})
	segments = append(segments, vaxis.Segment{Text: " ", Style: regularStyle})
	return segments
}

func splitStatusPathBase(text string) (string, string) {
	if text == "" {
		return "", ""
	}
	if space := strings.Index(text, " "); space >= 0 && isStatusCountPrefix(text[:space]) && space+1 < len(text) {
		return text[:space+1], text[space+1:]
	}
	if slash := strings.LastIndex(text, "/"); slash >= 0 && slash+1 < len(text) {
		return text[:slash+1], text[slash+1:]
	}
	return "", text
}

func isStatusCountPrefix(text string) bool {
	before, after, ok := strings.Cut(text, "/")
	if !ok || before == "" || after == "" {
		return false
	}
	for _, r := range before + after {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (d *diffViewer) statusStatsSegments(stats statusStats) []vaxis.Segment {
	return []vaxis.Segment{
		{Text: " ", Style: d.statusTextStyle()},
		{Text: fmt.Sprintf("+%d", stats.Adds), Style: d.statusAddStyle()},
		{Text: " ", Style: d.statusTextStyle()},
		{Text: fmt.Sprintf("-%d", stats.Deletes), Style: d.statusDeleteStyle()},
	}
}

func (d *diffViewer) currentStatusContext() statusContext {
	var context statusContext
	context.Commits = d.countRows(diff.RowCommitHeader)
	context.Files = d.countRows(diff.RowFile)
	context.TotalStats = rowsStats(d.rows)
	context.CommitIndex, context.Commit = d.currentCommitContext()
	context.FileIndex, context.File, context.FileStats = d.currentFileContext()
	return context
}

func (d *diffViewer) countRows(kind diff.RowKind) int {
	count := 0
	for _, row := range d.rows {
		if row.Kind == kind {
			count++
		}
	}
	return count
}

func (d *diffViewer) currentCommitContext() (int, string) {
	index := 0
	currentIndex := 0
	currentCommit := ""
	for rowIndex, row := range d.rows {
		if row.Kind != diff.RowCommitHeader {
			continue
		}
		index++
		if rowIndex <= d.cursor.Row {
			currentIndex = index
			currentCommit = row.Code
			if currentCommit == "" {
				currentCommit = strings.TrimPrefix(row.Text, "commit ")
			}
		}
	}
	return currentIndex, currentCommit
}

func (d *diffViewer) currentFileContext() (int, string, statusStats) {
	fileStart := -1
	fileIndex := 0
	currentIndex := 0
	fileName := ""
	for rowIndex, row := range d.rows {
		switch row.Kind {
		case diff.RowCommitHeader:
			if rowIndex <= d.cursor.Row {
				fileStart = -1
				currentIndex = 0
				fileName = ""
			}
		case diff.RowFile:
			fileIndex++
			if rowIndex <= d.cursor.Row {
				fileStart = rowIndex
				currentIndex = fileIndex
				fileName = row.Text
			}
		}
	}
	if fileStart < 0 {
		return 0, "", statusStats{}
	}
	fileEnd := len(d.rows)
	for rowIndex := fileStart + 1; rowIndex < len(d.rows); rowIndex++ {
		switch d.rows[rowIndex].Kind {
		case diff.RowFile, diff.RowCommitHeader:
			fileEnd = rowIndex
			break
		}
		if fileEnd == rowIndex {
			break
		}
	}
	return currentIndex, fileName, rowsStats(d.rows[fileStart:fileEnd])
}

func rowsStats(rows []diff.Row) statusStats {
	var stats statusStats
	for _, row := range rows {
		switch row.Kind {
		case diff.RowAdd:
			stats.Adds++
		case diff.RowDelete:
			stats.Deletes++
		}
	}
	return stats
}

func (s statusStats) String() string {
	return fmt.Sprintf("+%d -%d", s.Adds, s.Deletes)
}

func (d *diffViewer) paintCommandCursor(win vaxis.Window) {
	col, row, ok := d.commandCursorPosition(win)
	if !ok {
		return
	}
	if win.Vx != nil {
		win.ShowCursor(col, row, vaxis.CursorBeam)
	}
}

func (d *diffViewer) paintSearchCursor(win vaxis.Window) {
	col, row, ok := d.searchCursorPosition(win)
	if !ok {
		return
	}
	if win.Vx != nil {
		win.ShowCursor(col, row, vaxis.CursorBeam)
	}
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
	layout, ok := d.commentEditorLayout(width, height)
	if !ok {
		return
	}
	d.editor.ensureCursorVisible(layout.inputWidth, layout.visibleRows)

	inputStyle := vaxis.Style{
		Foreground: d.scheme.Foreground,
		Background: blendRGB(d.scheme.Background, d.scheme.Foreground, 0.08),
	}
	borderStyle := inputStyle
	borderStyle.Foreground = d.scheme.Muted

	d.paintCommentBorder(win, layout.x, layout.y, layout.boxWidth, layout.boxHeight, borderStyle)
	for row := 0; row < layout.visibleRows; row++ {
		screenRow := layout.y + 1 + row
		for col := 0; col < layout.boxWidth-2; col++ {
			win.SetCell(layout.x+1+col, screenRow, vaxis.Cell{
				Character: vaxis.Character{Grapheme: " ", Width: 1},
				Style:     inputStyle,
			})
		}
		wrappedRow := d.editor.scroll + row
		if wrappedRow < len(layout.wrapped) {
			printSegmentsAt(win.New(layout.x+1, screenRow, layout.inputWidth, 1), 0, 0, vaxis.Segment{
				Text:  layout.wrapped[wrappedRow].text(d.editor.lines),
				Style: inputStyle,
			})
		}
	}
	if layout.showScrollbar {
		d.paintCommentEditorScrollbar(win, layout, borderStyle)
	}

	d.paintCommentCursor(win, layout)
}

func (d *diffViewer) commentEditorRect(width int, height int) (int, int, int, int, bool) {
	layout, ok := d.commentEditorLayout(width, height)
	if !ok {
		return 0, 0, 0, 0, false
	}
	return layout.x, layout.y, layout.boxWidth, layout.boxHeight, true
}

func (d *diffViewer) commentEditorLayout(width int, height int) (commentEditorLayout, bool) {
	if d.editor == nil || width <= 0 || height <= 2 {
		return commentEditorLayout{}, false
	}

	screenCol, screenRow, ok := d.cursorScreenPositionForSize(width, height)
	if !ok {
		return commentEditorLayout{}, false
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
		return commentEditorLayout{}, false
	}

	x := screenCol
	if x+boxWidth > viewportWidth {
		x = viewportWidth - boxWidth
	}
	if x < 0 {
		x = 0
	}
	y := screenRow + 1

	inputWidth := boxWidth - 2
	if inputWidth < 1 {
		return commentEditorLayout{}, false
	}
	wrapped := d.editor.wrappedLines(inputWidth)
	visibleRows := commentEditorVisibleRows(len(wrapped), height)
	showScrollbar := len(wrapped) > visibleRows && inputWidth > 1
	if showScrollbar {
		inputWidth--
		wrapped = d.editor.wrappedLines(inputWidth)
		visibleRows = commentEditorVisibleRows(len(wrapped), height)
	}
	if visibleRows < 1 {
		return commentEditorLayout{}, false
	}
	return commentEditorLayout{
		x:             x,
		y:             y,
		boxWidth:      boxWidth,
		boxHeight:     visibleRows + 2,
		inputWidth:    inputWidth,
		visibleRows:   visibleRows,
		showScrollbar: showScrollbar,
		wrapped:       wrapped,
	}, true
}

func commentEditorVisibleRows(wrappedRows int, height int) int {
	rows := wrappedRows
	if rows < 1 {
		rows = 1
	}
	rows = minInt(rows, commentEditorMaxRows)
	if maxRows := height - 2; rows > maxRows {
		rows = maxRows
	}
	return rows
}

func (d *diffViewer) paintCommentBorder(win vaxis.Window, x int, y int, width int, height int, style vaxis.Style) {
	if width < 2 || height < 2 {
		return
	}

	right := x + width - 1
	bottom := y + height - 1
	win.SetCell(x, y, vaxis.Cell{Character: vaxis.Character{Grapheme: "╭", Width: 1}, Style: style})
	win.SetCell(right, y, vaxis.Cell{Character: vaxis.Character{Grapheme: "╮", Width: 1}, Style: style})
	win.SetCell(x, bottom, vaxis.Cell{Character: vaxis.Character{Grapheme: "╰", Width: 1}, Style: style})
	win.SetCell(right, bottom, vaxis.Cell{Character: vaxis.Character{Grapheme: "╯", Width: 1}, Style: style})
	for col := x + 1; col < right; col++ {
		win.SetCell(col, y, vaxis.Cell{Character: vaxis.Character{Grapheme: "─", Width: 1}, Style: style})
		win.SetCell(col, bottom, vaxis.Cell{Character: vaxis.Character{Grapheme: "─", Width: 1}, Style: style})
	}
	for row := y + 1; row < bottom; row++ {
		win.SetCell(x, row, vaxis.Cell{Character: vaxis.Character{Grapheme: "│", Width: 1}, Style: style})
		win.SetCell(right, row, vaxis.Cell{Character: vaxis.Character{Grapheme: "│", Width: 1}, Style: style})
	}
}

func (d *diffViewer) paintCommentEditorScrollbar(win vaxis.Window, layout commentEditorLayout, style vaxis.Style) {
	if !layout.showScrollbar || len(layout.wrapped) <= layout.visibleRows {
		return
	}
	bar := scrollbar{
		Length: layout.visibleRows,
		Size:   maxInt(1, (layout.visibleRows*layout.visibleRows)/len(layout.wrapped)),
		Thumb:  0,
	}
	if maxOffset := len(layout.wrapped) - layout.visibleRows; maxOffset > 0 {
		bar.Thumb = (d.editor.scroll * (layout.visibleRows - bar.Size)) / maxOffset
	}
	col := layout.x + 1 + layout.inputWidth
	for row := bar.Thumb; row < bar.Thumb+bar.Size && row < layout.visibleRows; row++ {
		win.SetCell(col, layout.y+1+row, vaxis.Cell{
			Character: vaxis.Character{Grapheme: verticalScrollbarThumb, Width: 1},
			Style:     style,
		})
	}
}

func (d *diffViewer) paintCommentCursor(win vaxis.Window, layout commentEditorLayout) {
	visualRow, col, ok := d.editor.cursorDisplayPosition(layout.inputWidth)
	if !ok {
		return
	}
	screenRow := visualRow - d.editor.scroll
	if screenRow < 0 || screenRow >= layout.visibleRows {
		return
	}
	if col < 0 || col >= layout.inputWidth {
		return
	}
	if win.Vx != nil {
		win.ShowCursor(layout.x+1+col, layout.y+1+screenRow, vaxis.CursorBeam)
	}
}

func (d *diffViewer) statusFillStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Foreground,
		Background: d.statusBackground(),
	}
}

func (d *diffViewer) statusTextStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Foreground,
		Background: d.statusBackground(),
	}
}

func (d *diffViewer) statusDimStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Dim,
		Background: d.statusBackground(),
	}
}

func (d *diffViewer) statusAddStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Add,
		Background: d.statusBackground(),
		Attribute:  vaxis.AttrBold,
	}
}

func (d *diffViewer) statusDeleteStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.scheme.Delete,
		Background: d.statusBackground(),
		Attribute:  vaxis.AttrBold,
	}
}

func (d *diffViewer) statusBackground() vaxis.Color {
	return blendRGB(d.scheme.Background, d.scheme.Foreground, 0.08)
}

func (d *diffViewer) statusCommitBackground() vaxis.Color {
	return blendRGB(d.statusBackground(), d.scheme.Base.Blue, 0.28)
}

func (d *diffViewer) statusFileBackground() vaxis.Color {
	return blendRGB(d.statusBackground(), d.scheme.Base.Cyan, 0.45)
}

func (d *diffViewer) statusStyle() vaxis.Style {
	return vaxis.Style{
		Foreground: d.statusBackground(),
		Background: d.statusColor(),
		Attribute:  vaxis.AttrBold,
	}
}

func (d *diffViewer) statusSeparatorStyle(background vaxis.Color) vaxis.Style {
	return vaxis.Style{
		Foreground: d.statusColor(),
		Background: background,
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
	if d.scroll >= 0 && d.scroll < len(d.rows) && !stickyFileHeaderAllowed(d.rows[d.scroll].Kind) {
		return diff.Row{}, false
	}
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

func stickyFileHeaderAllowed(kind diff.RowKind) bool {
	switch kind {
	case diff.RowCommitHeader, diff.RowCommitMeta, diff.RowCommitMessage, diff.RowCommitTrailer, diff.RowBlank:
		return false
	default:
		return true
	}
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

func (d *diffViewer) toggleLayoutMode() {
	if d.layoutMode == layoutSideBySide {
		d.layoutMode = layoutStacked
		return
	}
	d.layoutMode = layoutSideBySide
}

func (d *diffViewer) paintSideBySide(win vaxis.Window) {
	rows := d.sideBySideRows()
	start := d.sideBySideStart(rows)
	visible := d.visibleRowCapacity()
	if visible <= 0 {
		return
	}
	end := minInt(len(rows), start+visible)
	leftWidth, rightStart, rightWidth := d.sideBySidePaneGeometry(win)
	separatorStyle := vaxis.Style{Foreground: d.scheme.Muted, Background: d.scheme.Background}

	for screenRow, sideRow := range rows[start:end] {
		if sideRow.Full >= 0 {
			docRow := sideRow.Full
			d.printRow(win, screenRow, docRow, d.rows[docRow], d.codeSegments[docRow], docRow == d.cursor.Row)
			continue
		}
		if leftWidth > 0 {
			d.printSideBySideCell(win.New(0, screenRow, leftWidth, 1), sideRow.Left, sideLeft, sideRow.Left == d.cursor.Row)
		}
		if rightStart > 0 {
			win.SetCell(rightStart-1, screenRow, vaxis.Cell{
				Character: vaxis.Character{Grapheme: " ", Width: 1},
				Style:     separatorStyle,
			})
		}
		if rightWidth > 0 {
			d.printSideBySideCell(win.New(rightStart, screenRow, rightWidth, 1), sideRow.Right, sideRight, sideRow.Right == d.cursor.Row)
		}
	}
}

type diffSide int

const (
	sideLeft diffSide = iota
	sideRight
)

func (d *diffViewer) printSideBySideCell(win vaxis.Window, docRow int, side diffSide, cursorLine bool) {
	if docRow < 0 || docRow >= len(d.rows) {
		return
	}
	row := d.rows[docRow]
	gutter := d.sideBySideGutter(row, side)
	gutterSegments := d.rowSegments([]vaxis.Segment{{Text: gutter, Style: d.gutterStyle(row.Kind)}}, cursorLine)
	codeOffset := segmentTextWidth(gutterSegments)
	d.fillCodeBackground(win, 0, codeOffset, row.Kind, cursorLine)
	printSegmentsAt(win, 0, 0, gutterSegments...)
	if row.Code == "" {
		d.paintSideBySideSelection(win, docRow, row, codeOffset)
		return
	}
	codeSegments := d.reviewSegments(row, d.codeSegmentsForRow(docRow))
	codeSegments = d.searchSegments(docRow, row, codeSegments)
	printCodeSegmentsAtOffset(win, codeOffset, 0, d.xScroll, d.rowSegments(codeSegments, cursorLine)...)
	d.paintSideBySideSelection(win, docRow, row, codeOffset)
}

func (d *diffViewer) paintSideBySideSelection(win vaxis.Window, docRow int, row diff.Row, codeOffset int) {
	width, _ := win.Size()
	d.paintSideBySideSelectionCells(win, width, docRow, row, codeOffset)
}

func (d *diffViewer) paintSideBySideSelectionCells(dst cellSetter, width int, docRow int, row diff.Row, codeOffset int) {
	spec, ok := d.selectionPaintSpec(docRow, time.Now())
	if !ok {
		return
	}

	rowCodeOffset := d.codeOffset(row)
	if spec.endCol <= rowCodeOffset {
		return
	}
	if spec.startCol < rowCodeOffset {
		spec.startCol = rowCodeOffset
	}

	for screenCol := codeOffset; screenCol < width; screenCol++ {
		docCol := rowCodeOffset + d.xScroll + screenCol - codeOffset
		if docCol < spec.startCol || docCol >= spec.endCol {
			continue
		}
		dst.SetCell(screenCol, 0, vaxis.Cell{
			Character: characterAtCell(row.Code, docCol-rowCodeOffset),
			Style:     d.selectionCellStyle(row, docCol, spec.style),
		})
	}
}

func (d *diffViewer) sideBySidePaneGeometry(win vaxis.Window) (leftWidth int, rightStart int, rightWidth int) {
	width, _ := win.Size()
	leftWidth = width / 2
	if width > 1 {
		leftWidth = (width - 1) / 2
	}
	rightStart = leftWidth
	if width > 1 {
		rightStart++
	}
	rightWidth = width - rightStart
	if rightWidth < 0 {
		rightWidth = 0
	}
	return leftWidth, rightStart, rightWidth
}

func (d *diffViewer) sideBySideRows() []sideBySideRow {
	rows := make([]sideBySideRow, 0, len(d.rows))
	contextLeft := true
	contextRight := true
	for i := 0; i < len(d.rows); {
		if d.rows[i].Kind == diff.RowDelete {
			deleteStart := i
			for i < len(d.rows) && d.rows[i].Kind == diff.RowDelete {
				i++
			}
			addStart := i
			for i < len(d.rows) && d.rows[i].Kind == diff.RowAdd {
				i++
			}
			for deleteOffset, addOffset := 0, 0; deleteOffset < addStart-deleteStart || addOffset < i-addStart; {
				row := sideBySideRow{Full: -1, Left: -1, Right: -1}
				if deleteOffset < addStart-deleteStart {
					row.Left = deleteStart + deleteOffset
					deleteOffset++
				}
				if addOffset < i-addStart {
					row.Right = addStart + addOffset
					addOffset++
				}
				rows = append(rows, row)
			}
			continue
		}
		switch d.rows[i].Kind {
		case diff.RowHunk:
			contextLeft, contextRight = sideBySideHunkContextSides(d.rows, i)
			rows = append(rows, sideBySideRow{Full: i, Left: -1, Right: -1})
		case diff.RowAdd:
			rows = append(rows, sideBySideRow{Full: -1, Left: -1, Right: i})
		case diff.RowContext:
			row := sideBySideRow{Full: -1, Left: -1, Right: -1}
			if contextLeft {
				row.Left = i
			}
			if contextRight {
				row.Right = i
			}
			rows = append(rows, row)
		default:
			contextLeft = true
			contextRight = true
			rows = append(rows, sideBySideRow{Full: i, Left: -1, Right: -1})
		}
		i++
	}
	return rows
}

func sideBySideHunkContextSides(rows []diff.Row, hunk int) (bool, bool) {
	hasDeletes := false
	hasAdds := false
	for i := hunk + 1; i < len(rows); i++ {
		switch rows[i].Kind {
		case diff.RowDelete:
			hasDeletes = true
		case diff.RowAdd:
			hasAdds = true
		case diff.RowHunk, diff.RowFile, diff.RowMeta, diff.RowCommitHeader, diff.RowCommitMeta, diff.RowCommitMessage, diff.RowCommitTrailer, diff.RowBlank:
			return sideBySideContextSides(hasDeletes, hasAdds)
		}
	}
	return sideBySideContextSides(hasDeletes, hasAdds)
}

func sideBySideContextSides(hasDeletes bool, hasAdds bool) (bool, bool) {
	switch {
	case hasAdds && !hasDeletes:
		return false, true
	case hasDeletes && !hasAdds:
		return true, false
	default:
		return true, true
	}
}

func (d *diffViewer) sideBySideStart(rows []sideBySideRow) int {
	for index, row := range rows {
		if sideBySideRowFirstDoc(row) >= d.scroll {
			return index
		}
	}
	if len(rows) == 0 {
		return 0
	}
	return len(rows) - 1
}

func rowContainsDocRow(row sideBySideRow, docRow int) bool {
	return row.Full == docRow || row.Left == docRow || row.Right == docRow
}

func sideBySideRowFirstDoc(row sideBySideRow) int {
	first := -1
	for _, docRow := range []int{row.Full, row.Left, row.Right} {
		if docRow >= 0 && (first < 0 || docRow < first) {
			first = docRow
		}
	}
	return first
}

func (d *diffViewer) sideBySideCursorScreenPosition(width int, height int) (int, int, bool) {
	if d.cursor.Row < 0 || d.cursor.Row >= len(d.rows) {
		return 0, 0, false
	}
	rows := d.sideBySideRows()
	start := d.sideBySideStart(rows)
	visible := d.visibleRowCapacity()
	leftWidth, rightStart, _ := d.sideBySidePaneGeometry(vaxis.Window{Width: width, Height: height})
	for index := start; index < len(rows) && index < start+visible; index++ {
		if !rowContainsDocRow(rows[index], d.cursor.Row) {
			continue
		}
		if rows[index].Full == d.cursor.Row {
			screenCol := d.screenColumn(d.rows[d.cursor.Row], d.cursor.Col)
			if screenCol < 0 || screenCol >= width {
				return 0, 0, false
			}
			return screenCol, index - start, true
		}
		side := sideForRow(d.rows[d.cursor.Row])
		paneStart := 0
		paneWidth := leftWidth
		if side == sideRight {
			paneStart = rightStart
			paneWidth = width - rightStart
		}
		codeCol := d.cursor.Col - d.codeOffset(d.rows[d.cursor.Row])
		if codeCol < 0 {
			codeCol = 0
		}
		screenCol := paneStart + textCellWidth(d.sideBySideGutter(d.rows[d.cursor.Row], side)) + codeCol - d.xScroll
		if screenCol < paneStart || screenCol >= paneStart+paneWidth || screenCol >= width {
			return 0, 0, false
		}
		return screenCol, index - start, true
	}
	return 0, 0, false
}

func (d *diffViewer) sideBySideVisualIndex(rows []sideBySideRow, docRow int) (int, bool) {
	for index, row := range rows {
		if rowContainsDocRow(row, docRow) {
			return index, true
		}
	}
	return 0, false
}

func sideBySideDocRowForSide(row sideBySideRow, side diffSide) int {
	if row.Full >= 0 {
		return row.Full
	}
	if side == sideLeft {
		return firstAvailableDocRow(row.Left, row.Right)
	}
	return firstAvailableDocRow(row.Right, row.Left)
}

func firstAvailableDocRow(preferred int, fallback int) int {
	if preferred >= 0 {
		return preferred
	}
	return fallback
}

func sideForRow(row diff.Row) diffSide {
	if row.Kind == diff.RowDelete {
		return sideLeft
	}
	return sideRight
}

func (d *diffViewer) sideBySideGutter(row diff.Row, side diffSide) string {
	width := d.sideBySideLineNumberWidth()
	oldNumber, newNumber := splitGutterNumbers(row)
	number := ""
	marker := " "
	if side == sideLeft {
		number = oldNumber
		if row.Kind == diff.RowDelete {
			marker = "-"
		}
	} else {
		number = newNumber
		if row.Kind == diff.RowAdd {
			marker = "+"
		}
	}
	return fmt.Sprintf("%*s %s ", width, number, marker)
}

func (d *diffViewer) sideBySideLineNumberWidth() int {
	width := 1
	for _, row := range d.rows {
		oldNumber, newNumber := splitGutterNumbers(row)
		width = maxInt(width, len(oldNumber))
		width = maxInt(width, len(newNumber))
	}
	return width
}

func splitGutterNumbers(row diff.Row) (string, string) {
	fields := strings.Fields(row.Gutter)
	switch row.Kind {
	case diff.RowContext:
		if len(fields) >= 2 {
			return fields[0], fields[1]
		}
	case diff.RowDelete:
		if len(fields) >= 1 {
			return fields[0], ""
		}
	case diff.RowAdd:
		if len(fields) >= 1 {
			return "", fields[0]
		}
	}
	return "", ""
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
	contentViewportWidth := d.contentViewportWidthFor(width, verticalVisible)
	if width <= 0 || height <= 1 || trackWidth <= 0 {
		return scrollbar{}
	}

	contentWidth := d.contentWidth()
	if !horizontalVisible || contentWidth <= contentViewportWidth {
		return scrollbar{}
	}

	thumbSize := (contentViewportWidth * trackWidth) / contentWidth
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

func (d *diffViewer) selectTextObject(kind textObjectKind, object rune) bool {
	if d.cursor.Row < 0 || d.cursor.Row >= len(d.rows) {
		return false
	}
	if object == 'w' {
		return d.selectWordTextObject(kind)
	}
	open, close, ok := textObjectDelimiters(object)
	if !ok {
		return false
	}
	return d.selectDelimitedTextObject(kind, open, close)
}

func (d *diffViewer) selectWordTextObject(kind textObjectKind) bool {
	start, end := tokenRangeAt(d.rows[d.cursor.Row].Text, d.cursor.Col)
	codeStart, codeEnd, ok := d.codeRange(d.rows[d.cursor.Row])
	if !ok {
		return false
	}
	start = maxInt(start, codeStart)
	end = minInt(end, codeEnd)
	if kind == textObjectAround {
		end = d.extendAroundWord(d.cursor.Row, start, end, codeStart, codeEnd)
	}
	return d.applyTextObjectSelection(
		selectionPoint{Row: d.cursor.Row, Col: start},
		selectionPoint{Row: d.cursor.Row, Col: maxInt(start, end-1)},
	)
}

func (d *diffViewer) extendAroundWord(rowIndex int, start int, end int, codeStart int, codeEnd int) int {
	row := d.rows[rowIndex]
	for end < codeEnd && isSpaceRune(runeAtCell(row.Text, end)) {
		end++
	}
	if end == codeEnd {
		for start > codeStart && isSpaceRune(runeAtCell(row.Text, start-1)) {
			start--
		}
	}
	return end
}

func (d *diffViewer) selectDelimitedTextObject(kind textObjectKind, open rune, close rune) bool {
	bounds, ok := d.textObjectSearchBounds()
	if !ok {
		return false
	}
	cursor := textObjectPosition{
		Row: d.cursor.Row,
		Col: d.cursor.Col - bounds.CodeStart[d.cursor.Row],
	}
	if cursor.Col < 0 {
		cursor.Col = 0
	}
	if width := bounds.CodeWidth[d.cursor.Row]; cursor.Col >= width {
		cursor.Col = maxInt(0, width-1)
	}
	openPos, closePos, ok := findDelimitedTextObject(bounds, cursor, open, close)
	if !ok {
		return false
	}

	start := openPos
	end := closePos
	if kind == textObjectInner {
		start = advanceTextObjectPosition(bounds, openPos)
		end = previousTextObjectPosition(bounds, closePos)
	}
	if textObjectPositionLess(end, start) {
		return false
	}

	return d.applyTextObjectSelection(
		selectionPoint{Row: start.Row, Col: bounds.CodeStart[start.Row] + start.Col},
		selectionPoint{Row: end.Row, Col: bounds.CodeStart[end.Row] + end.Col},
	)
}

func (d *diffViewer) applyTextObjectSelection(anchor selectionPoint, cursor selectionPoint) bool {
	d.selection = textSelection{
		Active: true,
		Anchor: anchor,
		Cursor: cursor,
	}
	d.mode = modeVisual
	d.setCursor(cursor)
	return true
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
	if d.layoutMode == layoutSideBySide {
		return d.sideBySideSelectionPoint(mouse)
	}

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
	if d.layoutMode == layoutSideBySide {
		row, ok := d.sideBySideMouseDocumentRow(mouse)
		if !ok {
			return -1
		}
		return row
	}
	return d.scroll + mouse.Row
}

func (d *diffViewer) sideBySideSelectionPoint(mouse vaxis.Mouse) (selectionPoint, bool) {
	row, localCol, splitCodeOffset, ok := d.sideBySideMouseCell(mouse)
	if !ok || row < 0 || row >= len(d.rows) {
		return selectionPoint{}, false
	}

	start, end, ok := d.codeRange(d.rows[row])
	if !ok {
		return selectionPoint{}, false
	}
	docCol := d.codeOffset(d.rows[row]) + d.xScroll + localCol - splitCodeOffset
	if docCol < start {
		docCol = start
	}
	if docCol > end {
		docCol = end
	}
	return selectionPoint{Row: row, Col: docCol}, true
}

func (d *diffViewer) sideBySideMouseDocumentRow(mouse vaxis.Mouse) (int, bool) {
	row, _, _, ok := d.sideBySideMouseCell(mouse)
	return row, ok
}

func (d *diffViewer) sideBySideMouseCell(mouse vaxis.Mouse) (docRow int, localCol int, codeOffset int, ok bool) {
	if mouse.Row < 0 || mouse.Row >= d.visibleRowCapacity() {
		return 0, 0, 0, false
	}
	rows := d.sideBySideRows()
	start := d.sideBySideStart(rows)
	index := start + mouse.Row
	if index < 0 || index >= len(rows) {
		return 0, 0, 0, false
	}

	sideRow := rows[index]
	if sideRow.Full >= 0 {
		return sideRow.Full, mouse.Col, d.codeOffset(d.rows[sideRow.Full]), true
	}

	leftWidth, rightStart, rightWidth := d.sideBySidePaneGeometry(vaxis.Window{Width: d.width, Height: d.height})
	side := sideLeft
	paneStart := 0
	paneWidth := leftWidth
	switch {
	case mouse.Col < leftWidth:
		side = sideLeft
	case mouse.Col >= rightStart && mouse.Col < rightStart+rightWidth:
		side = sideRight
		paneStart = rightStart
		paneWidth = rightWidth
	default:
		return 0, 0, 0, false
	}
	if paneWidth <= 0 {
		return 0, 0, 0, false
	}

	docRow = sideBySideDocRowForSide(sideRow, side)
	if docRow < 0 || docRow >= len(d.rows) {
		return 0, 0, 0, false
	}
	localCol = mouse.Col - paneStart
	codeOffset = textCellWidth(d.sideBySideGutter(d.rows[docRow], side))
	return docRow, localCol, codeOffset, true
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
	if !selectableDiffRow(row.Kind) {
		return 0, 0, false
	}
	switch {
	case row.Gutter != "" || row.Marker != "":
		start := d.codeOffset(row)
		return start, start + textCellWidth(row.Code), true
	case row.Code != "":
		return 0, textCellWidth(row.Code), true
	default:
		return 0, 0, false
	}
}

func selectableDiffRow(kind diff.RowKind) bool {
	switch kind {
	case diff.RowContext, diff.RowAdd, diff.RowDelete:
		return true
	default:
		return false
	}
}

type textObjectBounds struct {
	Start     int
	End       int
	Side      diffSide
	Code      map[int]string
	CodeStart map[int]int
	CodeWidth map[int]int
}

type textObjectPosition struct {
	Row int
	Col int
}

func (d *diffViewer) textObjectSearchBounds() (textObjectBounds, bool) {
	if d.cursor.Row < 0 || d.cursor.Row >= len(d.rows) {
		return textObjectBounds{}, false
	}
	side := sideForRow(d.rows[d.cursor.Row])
	cursorFileName := d.rows[d.cursor.Row].FileName
	start := d.cursor.Row
	for start > 0 && textObjectRowsContiguous(d.rows[start-1], cursorFileName, side) {
		start--
	}
	end := d.cursor.Row
	for end+1 < len(d.rows) && textObjectRowsContiguous(d.rows[end+1], cursorFileName, side) {
		end++
	}

	bounds := textObjectBounds{
		Start:     start,
		End:       end,
		Side:      side,
		Code:      make(map[int]string, end-start+1),
		CodeStart: make(map[int]int, end-start+1),
		CodeWidth: make(map[int]int, end-start+1),
	}
	for row := start; row <= end; row++ {
		codeStart, codeEnd, ok := d.codeRange(d.rows[row])
		if !ok || !rowOnTextObjectSide(d.rows[row], side) {
			continue
		}
		bounds.Code[row] = d.rows[row].Code
		bounds.CodeStart[row] = codeStart
		bounds.CodeWidth[row] = codeEnd - codeStart
	}
	if _, ok := bounds.CodeStart[d.cursor.Row]; !ok {
		return textObjectBounds{}, false
	}
	return bounds, true
}

func textObjectRowsContiguous(row diff.Row, fileName string, side diffSide) bool {
	if !selectableDiffRow(row.Kind) || !rowOnTextObjectSide(row, side) {
		return false
	}
	return fileName == "" || row.FileName == fileName
}

func rowOnTextObjectSide(row diff.Row, side diffSide) bool {
	switch row.Kind {
	case diff.RowContext:
		return true
	case diff.RowDelete:
		return side == sideLeft
	case diff.RowAdd:
		return side == sideRight
	default:
		return false
	}
}

func (d *diffViewer) paintSelection(win vaxis.Window, screenRow int, docRow int) {
	spec, ok := d.selectionPaintSpec(docRow, time.Now())
	if !ok {
		return
	}

	width, _ := win.Size()
	if spec.startCol >= width {
		return
	}
	if spec.endCol > width && (spec.row.Code == "" || spec.row.Kind == diff.RowHunk) {
		spec.endCol = width
	}

	for screenCol := 0; screenCol < width; screenCol++ {
		docCol := d.documentColumn(spec.row, screenCol)
		if docCol >= spec.startCol && docCol < spec.endCol {
			win.SetCell(screenCol, screenRow, vaxis.Cell{
				Character: characterAtCell(spec.row.Text, docCol),
				Style:     d.selectionCellStyle(spec.row, docCol, spec.style),
			})
		}
	}
}

type selectionPaintSpec struct {
	row      diff.Row
	startCol int
	endCol   int
	style    vaxis.Style
}

func (d *diffViewer) selectionPaintSpec(docRow int, now time.Time) (selectionPaintSpec, bool) {
	start, end, ok := d.selectionRangeForPaint(now)
	if !ok || docRow < start.Row || docRow > end.Row {
		return selectionPaintSpec{}, false
	}
	startCol, endCol, ok := d.selectionPaintRange(docRow, start, end)
	if !ok {
		return selectionPaintSpec{}, false
	}
	return selectionPaintSpec{
		row:      d.rows[docRow],
		startCol: startCol,
		endCol:   endCol,
		style:    d.selectionStyleAt(now),
	}, true
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
	d.syncCommentEditorScroll()
	d.ensureCursorVisible()
	return true
}

func (d *diffViewer) handleCommentKey(key vaxis.Key) Command {
	command := CommandNone
	switch {
	case key.Matches(vaxis.KeyEsc), key.MatchString("Esc"):
		d.submitReviewComment()
		return CommandRedraw
	case key.Matches(vaxis.KeyEnter), key.Keycode == vaxis.KeyEnter:
		d.editor.insertLine()
		command = CommandRedraw
	case key.Matches(vaxis.KeyBackspace), key.Keycode == vaxis.KeyBackspace, key.Matches('h', vaxis.ModCtrl):
		d.editor.backspace()
		command = CommandRedraw
	case key.Matches(vaxis.KeyDelete):
		d.editor.deleteForward()
		command = CommandRedraw
	case key.Matches(vaxis.KeyLeft):
		d.editor.moveCol(-1)
		command = CommandRedraw
	case key.Matches(vaxis.KeyRight):
		d.editor.moveCol(1)
		command = CommandRedraw
	case key.Matches(vaxis.KeyUp):
		d.moveCommentEditorDisplayRow(-1)
		command = CommandRedraw
	case key.Matches(vaxis.KeyDown):
		d.moveCommentEditorDisplayRow(1)
		command = CommandRedraw
	case key.Text != "" && key.Modifiers&(vaxis.ModCtrl|vaxis.ModAlt|vaxis.ModSuper) == 0:
		if d.editor.insertText(key.Text) {
			command = CommandRedraw
		}
	}
	if command == CommandRedraw {
		d.syncCommentEditorScroll()
		d.ensureCursorVisible()
	}
	return command
}

func (d *diffViewer) moveCommentEditorDisplayRow(delta int) {
	layout, ok := d.commentEditorLayout(d.width, d.height)
	if !ok {
		d.editor.moveRow(delta)
		return
	}
	d.editor.moveDisplayRow(delta, layout.inputWidth)
}

func (d *diffViewer) syncCommentEditorScroll() {
	layout, ok := d.commentEditorLayout(d.width, d.height)
	if !ok {
		return
	}
	d.editor.ensureCursorVisible(layout.inputWidth, layout.visibleRows)
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

func (d *diffViewer) deleteReviewDraftAtTarget() bool {
	draft, ok := d.reviewDraftTarget()
	if !ok {
		return false
	}
	index, ok := d.findReviewDraft(draft)
	if !ok {
		return false
	}
	d.reviewDrafts = append(d.reviewDrafts[:index], d.reviewDrafts[index+1:]...)
	d.reviewDirty = true
	d.statusMessage = "Comment deleted."
	d.exitVisualMode()
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
		draft.CommitID != target.CommitID ||
		draft.OriginalCommitID != target.OriginalCommitID ||
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
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func (e *commentEditor) body() string {
	return strings.Join(e.lines, "\n")
}

func (e *commentEditor) wrappedLines(width int) []commentDisplayLine {
	if width < 1 {
		width = 1
	}
	wrapWidth := commentEditorWrapWidth(width)
	lines := make([]commentDisplayLine, 0, len(e.lines))
	for lineIndex, line := range e.lines {
		runes := []rune(line)
		if len(runes) == 0 {
			lines = append(lines, commentDisplayLine{line: lineIndex})
			continue
		}
		for start := 0; start < len(runes); {
			end := wrappedLineEnd(runes, start, wrapWidth)
			lines = append(lines, commentDisplayLine{line: lineIndex, start: start, end: end})
			start = end
		}
	}
	if len(lines) == 0 {
		return []commentDisplayLine{{}}
	}
	return lines
}

func commentEditorWrapWidth(width int) int {
	if width <= 1 {
		return 1
	}
	return width - 1
}

func wrappedLineEnd(runes []rune, start int, width int) int {
	col := 0
	end := start
	lastSpace := -1
	for end < len(runes) {
		next := col + graphemeCellWidth(string(runes[end]))
		if next > width && end > start {
			break
		}
		col = next
		if uucode.IsSpace(runes[end]) {
			lastSpace = end
		}
		end++
		if col >= width {
			break
		}
	}
	if end == start {
		return start + 1
	}
	if end < len(runes) && lastSpace >= start {
		return lastSpace + 1
	}
	return end
}

func (line commentDisplayLine) text(lines []string) string {
	if line.line < 0 || line.line >= len(lines) {
		return ""
	}
	runes := []rune(lines[line.line])
	if line.start < 0 {
		line.start = 0
	}
	if line.end > len(runes) {
		line.end = len(runes)
	}
	if line.end < line.start {
		line.end = line.start
	}
	return string(runes[line.start:line.end])
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

func (e *commentEditor) moveDisplayRow(delta int, width int) {
	lines := e.wrappedLines(width)
	index, col, ok := e.cursorDisplayPosition(width)
	if !ok {
		e.moveRow(delta)
		return
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(lines) {
		index = len(lines) - 1
	}
	target := lines[index]
	e.row = target.line
	e.col = target.start + runeColumnAtCell(target.text(e.lines), col)
	if e.col > target.end {
		e.col = target.end
	}
}

func (e *commentEditor) ensureCursorVisible(width int, visibleRows int) {
	lines := e.wrappedLines(width)
	index, _, ok := e.cursorDisplayPosition(width)
	if !ok {
		e.scroll = 0
		return
	}
	if visibleRows < 1 {
		visibleRows = 1
	}
	if e.scroll > index {
		e.scroll = index
	}
	if e.scroll+visibleRows <= index {
		e.scroll = index - visibleRows + 1
	}
	maxScroll := len(lines) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if e.scroll > maxScroll {
		e.scroll = maxScroll
	}
	if e.scroll < 0 {
		e.scroll = 0
	}
}

func (e *commentEditor) cursorDisplayPosition(width int) (int, int, bool) {
	lines := e.wrappedLines(width)
	for index, line := range lines {
		if line.line != e.row {
			continue
		}
		if e.col < line.start || e.col > line.end {
			continue
		}
		text := line.text(e.lines)
		runes := []rune(text)
		col := textCellWidth(string(runes[:minInt(len(runes), e.col-line.start)]))
		if col >= width {
			col = width - 1
		}
		if col < 0 {
			col = 0
		}
		return index, col, true
	}
	return 0, 0, false
}

func runeColumnAtCell(text string, target int) int {
	col := 0
	index := 0
	it := uucode.NewGraphemeIterator(text)
	for g, ok := it.Next(); ok; g, ok = it.Next() {
		cluster := text[g.Start:g.End]
		next := col + graphemeCellWidth(cluster)
		if target < next {
			return index
		}
		col = next
		index += utf8.RuneCountInString(cluster)
	}
	return index
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
	if startAnchor.CommitID != endAnchor.CommitID || startAnchor.OriginalCommitID != endAnchor.OriginalCommitID {
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
		if reviewDraftContains(draft, anchor) {
			return true
		}
	}
	return false
}

func reviewDraftContains(draft review.CommentDraft, anchor review.Anchor) bool {
	if draft.Path != anchor.Path ||
		draft.CommitID != anchor.CommitID ||
		draft.OriginalCommitID != anchor.OriginalCommitID {
		return false
	}
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
	if d.layoutMode == layoutSideBySide {
		d.moveSideBySideCursorRows(delta)
		return
	}

	d.prepareCursorForMovement()
	d.cursor.Row += delta
	d.clampCursor()
	d.cursor.Col = d.clampCursorCol(d.cursor.Row, d.cursorGoal)
	d.ensureCursorVisible()
	d.updateVisualSelection()
}

func (d *diffViewer) moveSideBySideCursorRows(delta int) {
	if len(d.rows) == 0 || delta == 0 {
		return
	}
	d.prepareCursorForMovement()
	rows := d.sideBySideRows()
	index, ok := d.sideBySideVisualIndex(rows, d.cursor.Row)
	if !ok {
		d.cursor.Row += delta
		d.clampCursor()
		d.cursor.Col = d.clampCursorCol(d.cursor.Row, d.cursorGoal)
		d.ensureCursorVisible()
		d.updateVisualSelection()
		return
	}

	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(rows) {
		index = len(rows) - 1
	}

	side := sideForRow(d.rows[d.cursor.Row])
	row := sideBySideDocRowForSide(rows[index], side)
	if row < 0 || row >= len(d.rows) {
		return
	}
	d.cursor.Row = row
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

func (d *diffViewer) cursorLineStart() {
	d.prepareCursorForMovement()
	if start, _, ok := d.codeRange(d.rows[d.cursor.Row]); ok {
		d.cursor.Col = start
	} else {
		d.cursor.Col = 0
	}
	d.cursorGoal = d.cursor.Col
	d.ensureCursorVisible()
	d.updateVisualSelection()
}

func (d *diffViewer) cursorLineEnd() {
	d.prepareCursorForMovement()
	if start, end, ok := d.codeRange(d.rows[d.cursor.Row]); ok {
		d.cursor.Col = maxInt(start, end-1)
	} else {
		d.cursor.Col = maxInt(0, textCellWidth(d.rows[d.cursor.Row].Text)-1)
	}
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

func (d *diffViewer) jumpCommit(direction int) bool {
	if len(d.rows) == 0 {
		return false
	}
	if direction < 0 {
		for row := d.cursor.Row - 1; row >= 0; row-- {
			if d.rows[row].Kind == diff.RowCommitHeader {
				d.setCursorAtCommit(row)
				d.updateVisualSelection()
				return true
			}
		}
		return false
	}
	for row := d.cursor.Row + 1; row < len(d.rows); row++ {
		if d.rows[row].Kind == diff.RowCommitHeader {
			d.setCursorAtCommit(row)
			d.updateVisualSelection()
			return true
		}
	}
	return false
}

func (d *diffViewer) setCursorAtCommit(row int) {
	d.cursor = selectionPoint{Row: row, Col: 0}
	d.clampCursor()
	d.cursorGoal = d.cursor.Col
	d.scroll = row
	d.clampScroll()
	d.ensureCursorColumnVisible()
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

	editorHeight := d.commentEditorHeightForSize(d.width, d.height)
	if editorHeight == 0 {
		return
	}
	minScroll := d.cursor.Row + editorHeight + 1 - visible
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

	codeWidth := d.codeViewportWidth(row)
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

func (d *diffViewer) codeViewportWidth(row diff.Row) int {
	verticalVisible, _ := d.scrollbarVisibility(d.width, d.height)
	if d.layoutMode != layoutSideBySide {
		viewportWidth := horizontalViewportWidth(d.width, verticalVisible)
		return viewportWidth - d.codeOffset(row)
	}

	viewportWidth := horizontalViewportWidth(d.width, verticalVisible)
	leftWidth, _, rightWidth := d.sideBySidePaneGeometry(vaxis.Window{Width: viewportWidth, Height: d.height})
	if row.Kind == diff.RowDelete {
		return leftWidth - textCellWidth(d.sideBySideGutter(row, sideLeft))
	}
	return rightWidth - textCellWidth(d.sideBySideGutter(row, sideRight))
}

func (d *diffViewer) maxScroll() int {
	maxScroll := len(d.rows) - 1
	if visible := d.visibleRowCapacity(); visible > 0 {
		maxScroll = len(d.rows) - visible
		if d.editor != nil {
			editorHeight := d.commentEditorHeightForSize(d.width, d.height)
			editorScroll := d.cursor.Row + editorHeight + 1 - visible
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

func (d *diffViewer) commentEditorHeightForSize(width int, height int) int {
	if d.editor == nil || width <= 0 || height <= 2 {
		return 0
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
	inputWidth := boxWidth - 2
	if inputWidth < 1 {
		return 0
	}
	wrapped := d.editor.wrappedLines(inputWidth)
	visibleRows := commentEditorVisibleRows(len(wrapped), height)
	if len(wrapped) > visibleRows && inputWidth > 1 {
		inputWidth--
		wrapped = d.editor.wrappedLines(inputWidth)
		visibleRows = commentEditorVisibleRows(len(wrapped), height)
	}
	if visibleRows < 1 {
		return 0
	}
	return visibleRows + 2
}

func (d *diffViewer) visibleRowCapacity() int {
	_, horizontalVisible := d.scrollbarVisibility(d.width, d.height)
	return visibleRowCapacity(d.height, horizontalVisible)
}

func (d *diffViewer) scrollBy(delta int) {
	d.scroll += delta
	d.clampScroll()
	d.clampCursorToVisibleRows()
	d.cursorGoal = d.cursor.Col
}

func (d *diffViewer) clampCursorToVisibleRows() {
	visible := d.visibleRowCapacity()
	if visible <= 0 {
		d.clampCursor()
		return
	}
	if d.cursor.Row < d.scroll {
		d.cursor.Row = d.scroll
	}
	lastVisible := d.scroll + visible - 1
	if d.cursor.Row > lastVisible {
		d.cursor.Row = lastVisible
	}
	if _, ok := d.stickyFileHeader(); ok && d.cursor.Row == d.scroll && d.scroll > 0 && d.cursor.Row+1 < len(d.rows) {
		d.cursor.Row++
	}
	d.clampCursor()
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
	maxScroll := d.contentWidth() - d.contentViewportWidth(verticalVisible)
	if maxScroll < 0 {
		return 0
	}
	return maxScroll
}

func (d *diffViewer) scrollbarVisibility(width int, height int) (vertical bool, horizontal bool) {
	if width <= 0 || height <= 1 {
		return false, false
	}

	horizontal = d.contentWidth() > d.contentViewportWidthFor(width, false)
	vertical = len(d.rows) > visibleRowCapacity(height, horizontal)
	if !horizontal && vertical {
		horizontal = d.contentWidth() > d.contentViewportWidthFor(width, vertical)
		vertical = len(d.rows) > visibleRowCapacity(height, horizontal)
	}
	return vertical, horizontal
}

func (d *diffViewer) contentViewportWidth(verticalVisible bool) int {
	return d.contentViewportWidthFor(d.width, verticalVisible)
}

func (d *diffViewer) contentViewportWidthFor(width int, verticalVisible bool) int {
	viewportWidth := horizontalViewportWidth(width, verticalVisible)
	if d.layoutMode != layoutSideBySide {
		return viewportWidth
	}
	leftWidth, _, rightWidth := d.sideBySidePaneGeometry(vaxis.Window{Width: viewportWidth, Height: d.height})
	return maxInt(leftWidth, rightWidth)
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
		rowWidth := textCellWidth(row.Text)
		if d.layoutMode == layoutSideBySide && selectableDiffRow(row.Kind) {
			rowWidth = textCellWidth(d.sideBySideGutter(row, sideForRow(row))) + textCellWidth(row.Code)
		}
		if rowWidth > width {
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
		if draft.Path != anchor.Path ||
			draft.Line != anchor.Line ||
			draft.Side != anchor.Side ||
			draft.CommitID != anchor.CommitID ||
			draft.OriginalCommitID != anchor.OriginalCommitID {
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

func printSegmentsClipped(win vaxis.Window, col int, row int, width int, segments ...vaxis.Segment) {
	winWidth, height := win.Size()
	if width <= 0 || col >= winWidth || row >= height {
		return
	}
	if col+width > winWidth {
		width = winWidth - col
	}
	line := win.New(col, row, width, 1)
	line.PrintTruncate(0, segments...)
}

func segmentsWidth(segments []vaxis.Segment) int {
	width := 0
	for _, segment := range segments {
		width += textCellWidth(segment.Text)
	}
	return width
}

func countLabel(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
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

func runeAtCell(text string, target int) rune {
	return []rune(characterAtCell(text, target).Grapheme)[0]
}

func isSpaceRune(r rune) bool {
	return uucode.IsSpace(r)
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

func findDelimitedTextObject(bounds textObjectBounds, cursor textObjectPosition, open rune, close rune) (textObjectPosition, textObjectPosition, bool) {
	if open == close {
		return findQuoteTextObject(bounds, cursor, open)
	}
	openPos, ok := findOpeningDelimiter(bounds, cursor, open, close)
	if !ok {
		return textObjectPosition{}, textObjectPosition{}, false
	}
	closePos, ok := findClosingDelimiter(bounds, openPos, open, close)
	if !ok {
		return textObjectPosition{}, textObjectPosition{}, false
	}
	if textObjectPositionLess(cursor, openPos) || textObjectPositionLess(closePos, cursor) {
		return textObjectPosition{}, textObjectPosition{}, false
	}
	return openPos, closePos, true
}

func findQuoteTextObject(bounds textObjectBounds, cursor textObjectPosition, delimiter rune) (textObjectPosition, textObjectPosition, bool) {
	positions := make([]textObjectPosition, 0)
	for row := bounds.Start; row <= bounds.End; row++ {
		width, ok := bounds.CodeWidth[row]
		if !ok {
			continue
		}
		for col := 0; col < width; col++ {
			if runeAtCell(rowCode(bounds, row), col) == delimiter {
				positions = append(positions, textObjectPosition{Row: row, Col: col})
			}
		}
	}
	for i := 0; i+1 < len(positions); i += 2 {
		openPos := positions[i]
		closePos := positions[i+1]
		if !textObjectPositionLess(cursor, openPos) && !textObjectPositionLess(closePos, cursor) {
			return openPos, closePos, true
		}
	}
	return textObjectPosition{}, textObjectPosition{}, false
}

func findOpeningDelimiter(bounds textObjectBounds, cursor textObjectPosition, open rune, close rune) (textObjectPosition, bool) {
	if open == close {
		return findPreviousQuoteDelimiter(bounds, cursor, open)
	}

	depth := 0
	for pos, ok := previousTextObjectScanPosition(bounds, cursor); ok; pos, ok = previousTextObjectScanPosition(bounds, beforeTextObjectPosition(pos)) {
		r := runeAtCell(rowCode(bounds, pos.Row), pos.Col)
		if r == close {
			depth++
			continue
		}
		if r != open {
			continue
		}
		if depth == 0 {
			return pos, true
		}
		depth--
	}
	return textObjectPosition{}, false
}

func findClosingDelimiter(bounds textObjectBounds, openPos textObjectPosition, open rune, close rune) (textObjectPosition, bool) {
	if open == close {
		return findNextMatchingDelimiter(bounds, openPos, close)
	}

	depth := 0
	for pos, ok := nextTextObjectScanPosition(bounds, openPos); ok; pos, ok = nextTextObjectScanPosition(bounds, afterTextObjectPosition(pos)) {
		r := runeAtCell(rowCode(bounds, pos.Row), pos.Col)
		if r == open && open != close {
			depth++
			continue
		}
		if r != close {
			continue
		}
		if depth == 0 {
			return pos, true
		}
		depth--
	}
	return textObjectPosition{}, false
}

func findPreviousMatchingDelimiter(bounds textObjectBounds, cursor textObjectPosition, delimiter rune) (textObjectPosition, bool) {
	for pos, ok := previousTextObjectScanPosition(bounds, cursor); ok; pos, ok = previousTextObjectScanPosition(bounds, beforeTextObjectPosition(pos)) {
		if runeAtCell(rowCode(bounds, pos.Row), pos.Col) == delimiter {
			return pos, true
		}
	}
	return textObjectPosition{}, false
}

func findPreviousQuoteDelimiter(bounds textObjectBounds, cursor textObjectPosition, delimiter rune) (textObjectPosition, bool) {
	positions := make([]textObjectPosition, 0)
	for row := bounds.Start; row <= cursor.Row; row++ {
		width, ok := bounds.CodeWidth[row]
		if !ok {
			continue
		}
		maxCol := width - 1
		if row == cursor.Row {
			maxCol = minInt(cursor.Col, width-1)
		}
		for col := 0; col <= maxCol; col++ {
			if runeAtCell(rowCode(bounds, row), col) == delimiter {
				positions = append(positions, textObjectPosition{Row: row, Col: col})
			}
		}
	}
	if len(positions) == 1 {
		return positions[len(positions)-1], true
	}
	if len(positions) >= 2 {
		return positions[len(positions)-2], true
	}
	return textObjectPosition{}, false
}

func findNextMatchingDelimiter(bounds textObjectBounds, openPos textObjectPosition, delimiter rune) (textObjectPosition, bool) {
	for pos, ok := nextTextObjectScanPosition(bounds, openPos); ok; pos, ok = nextTextObjectScanPosition(bounds, afterTextObjectPosition(pos)) {
		if runeAtCell(rowCode(bounds, pos.Row), pos.Col) == delimiter {
			return pos, true
		}
	}
	return textObjectPosition{}, false
}

func previousTextObjectScanPosition(bounds textObjectBounds, pos textObjectPosition) (textObjectPosition, bool) {
	for row := pos.Row; row >= bounds.Start; row-- {
		width, ok := bounds.CodeWidth[row]
		if !ok || width == 0 {
			continue
		}
		col := width - 1
		if row == pos.Row {
			col = minInt(pos.Col, width-1)
		}
		if col >= 0 {
			return textObjectPosition{Row: row, Col: col}, true
		}
	}
	return textObjectPosition{}, false
}

func nextTextObjectScanPosition(bounds textObjectBounds, pos textObjectPosition) (textObjectPosition, bool) {
	for row := pos.Row; row <= bounds.End; row++ {
		width, ok := bounds.CodeWidth[row]
		if !ok || width == 0 {
			continue
		}
		col := 0
		if row == pos.Row {
			col = pos.Col + 1
		}
		if col < 0 {
			col = 0
		}
		if col < width {
			return textObjectPosition{Row: row, Col: col}, true
		}
	}
	return textObjectPosition{}, false
}

func beforeTextObjectPosition(pos textObjectPosition) textObjectPosition {
	return textObjectPosition{Row: pos.Row, Col: pos.Col - 1}
}

func afterTextObjectPosition(pos textObjectPosition) textObjectPosition {
	return textObjectPosition{Row: pos.Row, Col: pos.Col + 1}
}

func advanceTextObjectPosition(bounds textObjectBounds, pos textObjectPosition) textObjectPosition {
	next, ok := nextTextObjectScanPosition(bounds, pos)
	if !ok {
		return pos
	}
	return next
}

func previousTextObjectPosition(bounds textObjectBounds, pos textObjectPosition) textObjectPosition {
	prev, ok := previousTextObjectScanPosition(bounds, textObjectPosition{Row: pos.Row, Col: pos.Col - 1})
	if !ok {
		return pos
	}
	return prev
}

func textObjectPositionLess(a textObjectPosition, b textObjectPosition) bool {
	if a.Row != b.Row {
		return a.Row < b.Row
	}
	return a.Col < b.Col
}

func textObjectDelimiters(object rune) (rune, rune, bool) {
	switch object {
	case '\'', '"', '`':
		return object, object, true
	case '(', ')':
		return '(', ')', true
	case '[', ']':
		return '[', ']', true
	case '{', '}':
		return '{', '}', true
	case '<', '>':
		return '<', '>', true
	default:
		return 0, 0, false
	}
}

func rowCode(bounds textObjectBounds, row int) string {
	return bounds.Code[row]
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
