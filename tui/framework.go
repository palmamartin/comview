package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"git.sr.ht/~rockorager/vaxis"
)

const (
	DefaultFrameRate = 60
	Unbounded        = -1
)

type Size struct {
	Width  int
	Height int
}

func (s Size) HasUnboundedWidth() bool {
	return s.Width == Unbounded
}

func (s Size) HasUnboundedHeight() bool {
	return s.Height == Unbounded
}

type Constraints struct {
	Min Size
	Max Size
}

func Tight(size Size) Constraints {
	return Constraints{
		Min: size,
		Max: size,
	}
}

func Loose(max Size) Constraints {
	return Constraints{
		Max: max,
	}
}

func Unconstrained() Constraints {
	return Loose(Size{
		Width:  Unbounded,
		Height: Unbounded,
	})
}

func (c Constraints) Constrain(size Size) Size {
	if size.Width < c.Min.Width {
		size.Width = c.Min.Width
	}
	if size.Height < c.Min.Height {
		size.Height = c.Min.Height
	}
	if !c.Max.HasUnboundedWidth() && size.Width > c.Max.Width {
		size.Width = c.Max.Width
	}
	if !c.Max.HasUnboundedHeight() && size.Height > c.Max.Height {
		size.Height = c.Max.Height
	}
	return size
}

type Command int

const (
	CommandNone Command = iota
	CommandRedraw
	CommandQuit
	CommandCopy
	CommandOpenEditor
)

type Widget interface {
	HandleEvent(vaxis.Event) (Command, error)
	Layout(Constraints) Size
	Paint(vaxis.Window)
}

type ClipboardProvider interface {
	ClipboardText() string
}

type ClipboardConsumer interface {
	ClipboardConsumed()
}

type EditorTarget struct {
	Path   string
	Line   int
	Column int
}

type EditorTargetProvider interface {
	EditorTarget() (EditorTarget, bool)
}

type StatusMessenger interface {
	SetStatusMessage(string)
}

type YankHighlighter interface {
	HighlightYank(time.Time)
	YankHighlightDuration() time.Duration
}

type TimedRedrawer interface {
	RedrawAfter() (time.Duration, bool)
}

type App struct {
	vx     *vaxis.Vaxis
	root   Widget
	frames FramePipeline
}

func NewApp(root Widget, opts vaxis.Options) (*App, error) {
	vx, err := vaxis.New(opts)
	if err != nil {
		return nil, err
	}

	return NewAppWithVaxis(root, vx), nil
}

func NewAppWithVaxis(root Widget, vx *vaxis.Vaxis) *App {
	return &App{
		vx:     vx,
		root:   root,
		frames: NewFramePipeline(DefaultFrameRate),
	}
}

func (a *App) Vaxis() *vaxis.Vaxis {
	return a.vx
}

func (a *App) Run() error {
	defer a.vx.Close()

	a.vx.SetTitle("comview")
	a.applyTerminalColors()
	a.frames.Request(a.draw)

	ticker := time.NewTicker(a.frames.Interval())
	defer ticker.Stop()

	events := a.vx.Events()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			cmd, err := a.handleEvent(ev)
			if err != nil {
				return err
			}
			if cmd == CommandQuit {
				return nil
			}
		case <-ticker.C:
			a.frames.Tick(a.draw)
		}
	}
}

func (a *App) draw() {
	win := a.vx.Window()
	width, height := win.Size()
	size := a.root.Layout(Tight(Size{Width: width, Height: height}))

	win.Clear()
	a.root.Paint(win.New(0, 0, size.Width, size.Height))
	a.vx.Render()
}

func (a *App) handleEvent(ev vaxis.Event) (Command, error) {
	requestFrame := false
	switch ev.(type) {
	case vaxis.Resize, vaxis.Redraw:
		requestFrame = true
	case vaxis.ColorThemeUpdate:
		a.applyTerminalColors()
		requestFrame = true
	}

	cmd, err := a.root.HandleEvent(ev)
	if err != nil {
		return CommandNone, err
	}

	if cmd == CommandCopy {
		if provider, ok := a.root.(ClipboardProvider); ok {
			if text := provider.ClipboardText(); text != "" {
				a.vx.ClipboardPush(text)
			}
		}
		if highlighter, ok := a.root.(YankHighlighter); ok {
			duration := highlighter.YankHighlightDuration()
			highlighter.HighlightYank(time.Now())
			go func() {
				time.Sleep(duration)
				a.vx.PostEvent(vaxis.Redraw{})
			}()
		}
		if consumer, ok := a.root.(ClipboardConsumer); ok {
			consumer.ClipboardConsumed()
		}
		requestFrame = true
	}
	if cmd == CommandOpenEditor {
		requestFrame = true
		if err := a.openEditor(); err != nil {
			if messenger, ok := a.root.(StatusMessenger); ok {
				messenger.SetStatusMessage(fmt.Sprintf("Could not open editor: %v", err))
			} else {
				return CommandNone, err
			}
		}
	}
	if cmd == CommandRedraw {
		requestFrame = true
	}
	if requestFrame {
		a.frames.Request(a.draw)
		a.scheduleTimedRedraw()
	}
	return cmd, nil
}

func (a *App) openEditor() error {
	provider, ok := a.root.(EditorTargetProvider)
	if !ok {
		return nil
	}
	target, ok := provider.EditorTarget()
	if !ok {
		return nil
	}
	if target.Path == "" {
		return nil
	}

	if err := a.vx.Suspend(); err != nil {
		return err
	}
	runErr := runEditor(target)
	resumeErr := a.vx.Resume()
	a.applyTerminalColors()
	if runErr != nil {
		return runErr
	}
	return resumeErr
}

func runEditor(target EditorTarget) error {
	editor := configuredEditor()
	name, args, err := editorCommand(editor, target)
	if err != nil {
		return err
	}

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = tty.Close() }()

	cmd := exec.Command(name, args...)
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	return cmd.Run()
}

func configuredEditor() string {
	if editor := strings.TrimSpace(os.Getenv("GIT_EDITOR")); editor != "" {
		return editor
	}
	if editor, ok := gitEditor(); ok {
		return editor
	}
	if editor := strings.TrimSpace(os.Getenv("VISUAL")); editor != "" {
		return editor
	}
	return strings.TrimSpace(os.Getenv("EDITOR"))
}

func gitEditor() (string, bool) {
	output, err := exec.Command("git", "var", "GIT_EDITOR").Output()
	if err != nil {
		return "", false
	}
	editor := strings.TrimSpace(string(output))
	return editor, editor != ""
}

func editorCommand(editor string, target EditorTarget) (string, []string, error) {
	if strings.TrimSpace(editor) == "" {
		editor = "vi"
	}
	parts, err := splitCommandLine(editor)
	if err != nil {
		return "", nil, err
	}
	if len(parts) == 0 {
		parts = []string{"vi"}
	}

	line := target.Line
	if line <= 0 {
		line = 1
	}
	column := target.Column
	if column <= 0 {
		column = 1
	}
	name := parts[0]
	args := append([]string{}, parts[1:]...)
	switch filepath.Base(name) {
	case "vi", "vim", "nvim", "view", "nano", "emacs", "emacsclient":
		args = append(args, fmt.Sprintf("+call cursor(%d,%d)", line, column), target.Path)
	case "code", "code-insiders", "codium", "vscodium":
		args = append(args, "-g", fmt.Sprintf("%s:%d:%d", target.Path, line, column))
	default:
		args = append(args, target.Path)
	}
	return name, args, nil
}

func splitCommandLine(command string) ([]string, error) {
	var fields []string
	var current strings.Builder
	quote := rune(0)
	escaped := false
	haveField := false

	for _, r := range command {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
			haveField = true
		case r == '\\' && quote != '\'':
			escaped = true
			haveField = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			haveField = true
		case r == '\'' || r == '"':
			quote = r
			haveField = true
		case unicode.IsSpace(r):
			if haveField {
				fields = append(fields, current.String())
				current.Reset()
				haveField = false
			}
		default:
			current.WriteRune(r)
			haveField = true
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in editor command")
	}
	if haveField {
		fields = append(fields, current.String())
	}
	return fields, nil
}

func (a *App) scheduleTimedRedraw() {
	redrawer, ok := a.root.(TimedRedrawer)
	if !ok {
		return
	}
	duration, ok := redrawer.RedrawAfter()
	if !ok {
		return
	}
	go func() {
		time.Sleep(duration)
		a.vx.PostEvent(vaxis.Redraw{})
	}()
}

func (a *App) applyTerminalColors() {
	if receiver, ok := a.root.(TerminalColorReceiver); ok {
		receiver.SetTerminalColors(QueryTerminalColors(a.vx))
	}
}

type FramePipeline struct {
	interval time.Duration
	last     time.Time
	pending  bool
}

func NewFramePipeline(rate int) FramePipeline {
	if rate <= 0 {
		rate = DefaultFrameRate
	}

	return FramePipeline{
		interval: time.Second / time.Duration(rate),
	}
}

func (p *FramePipeline) Interval() time.Duration {
	return p.interval
}

func (p *FramePipeline) Request(render func()) {
	p.request(time.Now(), render)
}

func (p *FramePipeline) request(now time.Time, render func()) {
	if p.last.IsZero() || now.Sub(p.last) >= p.interval {
		p.render(now, render)
		return
	}

	p.pending = true
}

func (p *FramePipeline) Tick(render func()) {
	p.tick(time.Now(), render)
}

func (p *FramePipeline) tick(now time.Time, render func()) {
	if !p.pending {
		return
	}

	p.render(now, render)
}

func (p *FramePipeline) render(now time.Time, render func()) {
	p.last = now
	p.pending = false
	render()
}
