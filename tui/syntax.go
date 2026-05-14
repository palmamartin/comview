package tui

import (
	"fmt"
	"strings"

	"git.sr.ht/~rockorager/vaxis"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

type SyntaxHighlighter struct {
	style  *chroma.Style
	lexers map[string]chroma.Lexer
}

func NewSyntaxHighlighter() *SyntaxHighlighter {
	return NewSyntaxHighlighterWithScheme(DefaultColorScheme())
}

func NewSyntaxHighlighterWithScheme(scheme ColorScheme) *SyntaxHighlighter {
	return &SyntaxHighlighter{
		style:  syntaxStyle(scheme),
		lexers: make(map[string]chroma.Lexer),
	}
}

func (h *SyntaxHighlighter) SetColorScheme(scheme ColorScheme) {
	h.style = syntaxStyle(scheme)
}

func (h *SyntaxHighlighter) Highlight(fileName string, code string, base vaxis.Style) []vaxis.Segment {
	if h == nil || code == "" {
		return []vaxis.Segment{{Text: code, Style: base}}
	}

	lexer := h.lexerFor(fileName)
	if lexer == nil || lexer == lexers.Fallback {
		return []vaxis.Segment{{Text: code, Style: base}}
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return []vaxis.Segment{{Text: code, Style: base}}
	}

	var segments []vaxis.Segment
	for token := iterator(); token != chroma.EOF; token = iterator() {
		text := strings.TrimSuffix(token.Value, "\n")
		if text == "" {
			continue
		}

		segments = append(segments, vaxis.Segment{
			Text:  text,
			Style: h.styleFor(token.Type, base),
		})
	}

	if len(segments) == 0 {
		return []vaxis.Segment{{Text: code, Style: base}}
	}
	return segments
}

func (h *SyntaxHighlighter) lexerFor(fileName string) chroma.Lexer {
	if lexer, ok := h.lexers[fileName]; ok {
		return lexer
	}

	lexer := lexers.Match(fileName)
	if lexer != nil {
		lexer = chroma.Coalesce(lexer)
	}
	h.lexers[fileName] = lexer
	return lexer
}

func (h *SyntaxHighlighter) styleFor(tokenType chroma.TokenType, base vaxis.Style) vaxis.Style {
	style := base
	entry := h.style.Get(tokenType)
	if entry.Colour.IsSet() {
		style.Foreground = vaxis.RGBColor(entry.Colour.Red(), entry.Colour.Green(), entry.Colour.Blue())
	}
	if entry.Bold == chroma.Yes {
		style.Attribute |= vaxis.AttrBold
	}
	if entry.Italic == chroma.Yes {
		style.Attribute |= vaxis.AttrItalic
	}
	if entry.Underline == chroma.Yes {
		style.UnderlineStyle = vaxis.UnderlineSingle
	}
	return style
}

func syntaxStyle(scheme ColorScheme) *chroma.Style {
	entries := chroma.StyleEntries{
		chroma.Text:              chromaColor(scheme.Foreground),
		chroma.Keyword:           chromaColor(scheme.Magenta()),
		chroma.KeywordType:       chromaColor(scheme.Cyan()),
		chroma.KeywordConstant:   chromaColor(scheme.Yellow),
		chroma.NameBuiltin:       chromaColor(scheme.Cyan()),
		chroma.NameClass:         chromaColor(scheme.Yellow),
		chroma.NameFunction:      chromaColor(scheme.Blue),
		chroma.NameAttribute:     chromaColor(scheme.Cyan()),
		chroma.NameVariable:      chromaColor(scheme.Foreground),
		chroma.LiteralString:     chromaColor(scheme.Green()),
		chroma.LiteralNumber:     chromaColor(scheme.Yellow),
		chroma.Operator:          chromaColor(scheme.Magenta()),
		chroma.Punctuation:       chromaColor(scheme.Muted),
		chroma.Comment:           chromaColor(scheme.Muted) + " italic",
		chroma.CommentPreproc:    chromaColor(scheme.Cyan()) + " italic",
		chroma.GenericDeleted:    chromaColor(scheme.Delete),
		chroma.GenericInserted:   chromaColor(scheme.Add),
		chroma.GenericHeading:    chromaColor(scheme.Header) + " bold",
		chroma.GenericSubheading: chromaColor(scheme.Hunk),
	}
	return chroma.MustNewStyle("comview", entries)
}

func chromaColor(color vaxis.Color) string {
	r, g, b := rgb(color)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}
