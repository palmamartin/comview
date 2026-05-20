package terminal

import (
	"io"
	"strings"

	"git.sr.ht/~rockorager/vaxis/ansi"
)

// PrintableANSIOutput returns the printable graphemes from terminal output,
// preserving line breaks, carriage returns, and tabs.
func PrintableANSIOutput(input io.Reader) string {
	parser := ansi.NewParser(input, ansi.ParserModeOutput)
	var output strings.Builder
	for seq := range parser.Next() {
		switch seq := seq.(type) {
		case ansi.Print:
			output.WriteString(seq.Grapheme)
		case ansi.C0:
			switch rune(seq) {
			case '\n', '\r', '\t':
				output.WriteRune(rune(seq))
			}
		}
	}
	return output.String()
}
