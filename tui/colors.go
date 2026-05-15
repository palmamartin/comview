package tui

import (
	"context"
	"math"
	"time"

	"git.sr.ht/~rockorager/vaxis"
)

const colorQueryTimeout = 150 * time.Millisecond

const (
	changedLineBlend        = 0.05
	minChangedLineContrast  = 1.18
	maxChangedLineBlendStep = 0.22
	inlineChangeBlend       = 0.32
	minInlineChangeContrast = 1.70
	dimBlend                = 0.55
	codeBackgroundBlend     = 0.035
	selectionBlend          = 0.40
	yankHighlightBlend      = 0.50
	cursorLineBlend         = 0.10
	gutterBackgroundBlend   = 0.16
)

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

type BaseColors struct {
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
	Base         BaseColors
	Foreground   vaxis.Color
	Background   vaxis.Color
	Code         vaxis.Color
	Dim          vaxis.Color
	Header       vaxis.Color
	Muted        vaxis.Color
	Hunk         vaxis.Color
	Gutter       vaxis.Color
	Blue         vaxis.Color
	Yellow       vaxis.Color
	Add          vaxis.Color
	AddLine      vaxis.Color
	AddInline    vaxis.Color
	Delete       vaxis.Color
	DeleteLine   vaxis.Color
	DeleteInline vaxis.Color
	Selection    vaxis.Color
	Yank         vaxis.Color
}

func DefaultBaseColors() BaseColors {
	return BaseColors{
		Foreground: vaxis.RGBColor(0xd7, 0xde, 0xe9),
		Background: vaxis.RGBColor(0x10, 0x14, 0x19),
		Red:        vaxis.RGBColor(0xe0, 0x6c, 0x75),
		Green:      vaxis.RGBColor(0x98, 0xc3, 0x79),
		Yellow:     vaxis.RGBColor(0xe5, 0xc0, 0x7b),
		Blue:       vaxis.RGBColor(0x61, 0xaf, 0xef),
		Magenta:    vaxis.RGBColor(0xc6, 0x78, 0xdd),
		Cyan:       vaxis.RGBColor(0x56, 0xb6, 0xc2),
	}
}

func DefaultColorScheme() ColorScheme {
	return NewColorScheme(DefaultBaseColors())
}

func NewColorScheme(base BaseColors) ColorScheme {
	scheme := ColorScheme{
		Base:       base,
		Foreground: base.Foreground,
		Background: base.Background,
		Header:     base.Cyan,
		Muted:      blendRGB(base.Foreground, base.Background, 0.44),
		Hunk:       base.Magenta,
		Blue:       base.Blue,
		Yellow:     base.Yellow,
		Add:        base.Green,
		Delete:     base.Red,
	}
	scheme.RecomputeDerivedColors()
	return scheme
}

func (s *ColorScheme) ApplyTerminalColors(colors TerminalColors) {
	if colors.Foreground != vaxis.ColorDefault {
		s.Base.Foreground = colors.Foreground
	}
	if colors.Background != vaxis.ColorDefault {
		s.Base.Background = colors.Background
	}
	if colors.Red != vaxis.ColorDefault {
		s.Base.Red = colors.Red
	}
	if colors.Green != vaxis.ColorDefault {
		s.Base.Green = colors.Green
	}
	if colors.Magenta != vaxis.ColorDefault {
		s.Base.Magenta = colors.Magenta
	}
	if colors.Cyan != vaxis.ColorDefault {
		s.Base.Cyan = colors.Cyan
	}
	if colors.Blue != vaxis.ColorDefault {
		s.Base.Blue = colors.Blue
	}
	if colors.Yellow != vaxis.ColorDefault {
		s.Base.Yellow = colors.Yellow
	}
	*s = NewColorScheme(s.Base)
}

func (s *ColorScheme) RecomputeDerivedColors() {
	s.Dim = blendRGB(s.Foreground, s.Background, dimBlend)
	s.Code = blendRGB(s.Background, s.Foreground, codeBackgroundBlend)
	s.AddLine = changedLineBackground(s.Background, s.Add)
	s.DeleteLine = changedLineBackground(s.Background, s.Delete)
	s.AddInline = inlineChangeBackground(s.Background, s.Add)
	s.DeleteInline = inlineChangeBackground(s.Background, s.Delete)
	s.Selection = blendRGB(s.Background, s.Blue, selectionBlend)
	s.Yank = blendRGB(s.Background, s.Yellow, yankHighlightBlend)
	s.Gutter = blendRGB(s.Background, gutterShadeTarget(s.Background), gutterBackgroundBlend)
}

func gutterShadeTarget(background vaxis.Color) vaxis.Color {
	if isLightColor(background) {
		return trueWhite()
	}
	return trueBlack()
}

func isLightColor(color vaxis.Color) bool {
	return relativeLuminance(color) >= 0.5
}

func trueBlack() vaxis.Color {
	return vaxis.RGBColor(0, 0, 0)
}

func trueWhite() vaxis.Color {
	return vaxis.RGBColor(0xff, 0xff, 0xff)
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

func changedLineBackground(background vaxis.Color, accent vaxis.Color) vaxis.Color {
	blend := changedLineBlend
	color := blendRGB(background, accent, blend)
	for contrastRatio(background, color) < minChangedLineContrast && blend < maxChangedLineBlendStep {
		blend += 0.04
		color = blendRGB(background, accent, blend)
	}
	return color
}

func inlineChangeBackground(background vaxis.Color, accent vaxis.Color) vaxis.Color {
	color := blendRGB(background, accent, inlineChangeBlend)
	for contrastRatio(background, color) < minInlineChangeContrast {
		color = blendRGB(background, accent, inlineChangeBlend+0.08)
		if contrastRatio(background, color) >= minInlineChangeContrast {
			break
		}
		color = blendRGB(background, accent, 0.50)
		break
	}
	return color
}

func blendRGB(base vaxis.Color, accent vaxis.Color, amount float64) vaxis.Color {
	br, bg, bb := rgb(base)
	ar, ag, ab := rgb(accent)

	return vaxis.RGBColor(
		blendChannel(br, ar, amount),
		blendChannel(bg, ag, amount),
		blendChannel(bb, ab, amount),
	)
}

func blendChannel(base uint8, accent uint8, amount float64) uint8 {
	value := float64(base) + (float64(accent)-float64(base))*amount
	return uint8(math.Round(value))
}

func contrastRatio(a vaxis.Color, b vaxis.Color) float64 {
	l1 := relativeLuminance(a)
	l2 := relativeLuminance(b)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func relativeLuminance(color vaxis.Color) float64 {
	r, g, b := rgb(color)
	return 0.2126*linearized(float64(r)/255) +
		0.7152*linearized(float64(g)/255) +
		0.0722*linearized(float64(b)/255)
}

func linearized(channel float64) float64 {
	if channel <= 0.03928 {
		return channel / 12.92
	}
	return math.Pow((channel+0.055)/1.055, 2.4)
}

func rgb(color vaxis.Color) (uint8, uint8, uint8) {
	params := color.Params()
	if len(params) != 3 {
		return 0, 0, 0
	}
	return params[0], params[1], params[2]
}

func (s ColorScheme) Cyan() vaxis.Color {
	return s.Header
}

func (s ColorScheme) Green() vaxis.Color {
	return s.Add
}

func (s ColorScheme) Magenta() vaxis.Color {
	return s.Hunk
}
