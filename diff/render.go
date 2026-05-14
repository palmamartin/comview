package diff

import (
	"fmt"
	"strings"
)

type RenderOptions struct {
	ShowPreamble         bool
	ShowFileHeaders      bool
	ShowFileMetadata     bool
	ShowHunkHeaders      bool
	ShowContext          bool
	ShowNoNewlineMarkers bool
	ShowLineNumbers      bool
	LineNumberWidth      int
}

func DefaultRenderOptions() RenderOptions {
	return RenderOptions{
		ShowPreamble:         true,
		ShowFileHeaders:      true,
		ShowFileMetadata:     true,
		ShowHunkHeaders:      true,
		ShowContext:          true,
		ShowNoNewlineMarkers: true,
		ShowLineNumbers:      true,
	}
}

func (d Document) Rows() []Row {
	return d.RowsWithOptions(DefaultRenderOptions())
}

func (d Document) RowsWithOptions(options RenderOptions) []Row {
	if options.LineNumberWidth <= 0 {
		options.LineNumberWidth = d.lineNumberWidth()
	}

	rows := make([]Row, 0)
	if options.ShowPreamble {
		for _, line := range d.Preamble {
			rows = append(rows, Row{Kind: RowPreamble, Text: line})
		}
	}

	for _, file := range d.Files {
		name := fileName(file)
		syntaxName := syntaxFileName(file)
		if options.ShowFileHeaders {
			rows = append(rows, Row{Kind: RowFile, Text: name, FileName: syntaxName})
		}

		if options.ShowFileMetadata {
			for _, line := range file.Header {
				if strings.HasPrefix(line, "diff --git ") {
					continue
				}
				rows = append(rows, Row{Kind: RowMeta, Text: line, FileName: syntaxName})
			}
		}

		for _, hunk := range file.Hunks {
			if options.ShowHunkHeaders {
				rows = append(rows, Row{Kind: RowHunk, Text: hunk.Header, FileName: syntaxName})
			}
			rows = append(rows, renderHunkRows(syntaxName, hunk, options)...)
		}
	}

	return rows
}

func renderHunkRows(fileName string, hunk Hunk, options RenderOptions) []Row {
	rows := make([]Row, 0, len(hunk.Lines))
	for i := 0; i < len(hunk.Lines); {
		if hunk.Lines[i].Kind == Delete {
			deleteStart := i
			for i < len(hunk.Lines) && hunk.Lines[i].Kind == Delete {
				i++
			}
			addStart := i
			for i < len(hunk.Lines) && hunk.Lines[i].Kind == Add {
				i++
			}

			deletes := hunk.Lines[deleteStart:addStart]
			adds := hunk.Lines[addStart:i]
			inlineDiffs := pairInlineDiffs(deletes, adds)

			for idx, line := range deletes {
				row := renderRow(fileName, line, options)
				row.InlineSpans = inlineDiffs.deleteSpans[idx]
				rows = append(rows, row)
			}
			for idx, line := range adds {
				row := renderRow(fileName, line, options)
				row.InlineSpans = inlineDiffs.addSpans[idx]
				rows = append(rows, row)
			}
			continue
		}

		line := hunk.Lines[i]
		i++
		if line.Kind == Context && !options.ShowContext {
			continue
		}
		if line.Kind == NoNewline && !options.ShowNoNewlineMarkers {
			continue
		}
		rows = append(rows, renderRow(fileName, line, options))
	}
	return rows
}

func renderRow(fileName string, line Line, options RenderOptions) Row {
	gutter := renderGutter(line, options)
	marker, code := splitLine(line)
	return Row{
		Kind:     rowKind(line.Kind),
		Text:     gutter + marker + code,
		FileName: fileName,
		Gutter:   gutter,
		Marker:   marker,
		Code:     code,
	}
}

func renderLine(line Line, options RenderOptions) string {
	return renderGutter(line, options) + line.Text
}

func renderGutter(line Line, options RenderOptions) string {
	if !options.ShowLineNumbers {
		return ""
	}

	oldNumber := lineNumber(line.OldLine)
	newNumber := lineNumber(line.NewLine)
	return fmt.Sprintf("%*s %*s │ ", options.LineNumberWidth, oldNumber, options.LineNumberWidth, newNumber)
}

func splitLine(line Line) (marker string, code string) {
	if line.Kind == NoNewline || line.Text == "" {
		return "", line.Text
	}
	return line.Text[:1], line.Text[1:]
}

func lineNumber(number int) string {
	if number == 0 {
		return ""
	}
	return fmt.Sprintf("%d", number)
}

func (d Document) lineNumberWidth() int {
	maxNumber := 0
	for _, file := range d.Files {
		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				if line.OldLine > maxNumber {
					maxNumber = line.OldLine
				}
				if line.NewLine > maxNumber {
					maxNumber = line.NewLine
				}
			}
		}
	}
	if maxNumber == 0 {
		return 1
	}
	return len(fmt.Sprintf("%d", maxNumber))
}
