package tui

import (
	"time"

	"git.sr.ht/~rockorager/vaxis"
)

const DefaultFrameRate = 60

type Size struct {
	Width  int
	Height int
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

func (c Constraints) Constrain(size Size) Size {
	if size.Width < c.Min.Width {
		size.Width = c.Min.Width
	}
	if size.Height < c.Min.Height {
		size.Height = c.Min.Height
	}
	if size.Width > c.Max.Width {
		size.Width = c.Max.Width
	}
	if size.Height > c.Max.Height {
		size.Height = c.Max.Height
	}
	return size
}

type Command int

const (
	CommandNone Command = iota
	CommandRedraw
	CommandQuit
)

type Widget interface {
	HandleEvent(vaxis.Event) (Command, error)
	Layout(Constraints) Size
	Paint(vaxis.Window)
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
	}

	cmd, err := a.root.HandleEvent(ev)
	if err != nil {
		return CommandNone, err
	}

	if cmd == CommandRedraw {
		requestFrame = true
	}
	if requestFrame {
		a.frames.Request(a.draw)
	}
	return cmd, nil
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
