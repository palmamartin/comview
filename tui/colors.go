package tui

import (
	"context"
	"time"

	"git.sr.ht/~rockorager/vaxis"
)

const colorQueryTimeout = 150 * time.Millisecond

type TerminalColors struct {
	Foreground vaxis.Color
	Background vaxis.Color
	Red        vaxis.Color
	Green      vaxis.Color
	Yellow     vaxis.Color
	Blue       vaxis.Color
	Magenta    vaxis.Color
	Cyan       vaxis.Color
}

type ColorScheme struct {
	Foreground vaxis.Color
	Background vaxis.Color
	Header     vaxis.Color
	Muted      vaxis.Color
	Hunk       vaxis.Color
	Add        vaxis.Color
	Delete     vaxis.Color
}

func DefaultColorScheme() ColorScheme {
	return ColorScheme{
		Foreground: vaxis.RGBColor(0xd7, 0xde, 0xe9),
		Background: vaxis.RGBColor(0x10, 0x14, 0x19),
		Header:     vaxis.RGBColor(0x56, 0xb6, 0xc2),
		Muted:      vaxis.RGBColor(0x7f, 0x88, 0x96),
		Hunk:       vaxis.RGBColor(0xc6, 0x78, 0xdd),
		Add:        vaxis.RGBColor(0x98, 0xc3, 0x79),
		Delete:     vaxis.RGBColor(0xe0, 0x6c, 0x75),
	}
}

func (s *ColorScheme) ApplyTerminalColors(colors TerminalColors) {
	if colors.Foreground != vaxis.ColorDefault {
		s.Foreground = colors.Foreground
	}
	if colors.Background != vaxis.ColorDefault {
		s.Background = colors.Background
	}
	if colors.Red != vaxis.ColorDefault {
		s.Delete = colors.Red
	}
	if colors.Green != vaxis.ColorDefault {
		s.Add = colors.Green
	}
	if colors.Magenta != vaxis.ColorDefault {
		s.Hunk = colors.Magenta
	}
	if colors.Cyan != vaxis.ColorDefault {
		s.Header = colors.Cyan
	}
}

type TerminalColorReceiver interface {
	SetTerminalColors(TerminalColors)
}

func QueryTerminalColors(vx *vaxis.Vaxis) TerminalColors {
	return TerminalColors{
		Foreground: queryTerminalColor(vx.CanReportForegroundColor(), vx.QueryForeground),
		Background: queryTerminalBackground(vx),
		Red:        queryIndexedTerminalColor(vx, 1),
		Green:      queryIndexedTerminalColor(vx, 2),
		Yellow:     queryIndexedTerminalColor(vx, 3),
		Blue:       queryIndexedTerminalColor(vx, 4),
		Magenta:    queryIndexedTerminalColor(vx, 5),
		Cyan:       queryIndexedTerminalColor(vx, 6),
	}
}

func queryTerminalBackground(vx *vaxis.Vaxis) vaxis.Color {
	if !vx.CanReportBackgroundColor() {
		return vaxis.ColorDefault
	}

	ctx, cancel := context.WithTimeout(context.Background(), colorQueryTimeout)
	defer cancel()
	return vx.QueryBackgroundContext(ctx)
}

func queryTerminalColor(supported bool, query func() vaxis.Color) vaxis.Color {
	if !supported {
		return vaxis.ColorDefault
	}

	ch := make(chan vaxis.Color, 1)
	go func() {
		ch <- query()
	}()

	select {
	case color := <-ch:
		return color
	case <-time.After(colorQueryTimeout):
		return vaxis.ColorDefault
	}
}

func queryIndexedTerminalColor(vx *vaxis.Vaxis, index uint8) vaxis.Color {
	return queryTerminalColor(vx.CanReportColor(), func() vaxis.Color {
		return vx.QueryColor(vaxis.IndexColor(index))
	})
}
