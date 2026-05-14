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

	for fileIndex, file := range d.Files {
		name := fileName(file)
		syntaxName := syntaxFileName(file)
		if options.ShowFileHeaders {
			if fileIndex > 0 {
				rows = append(rows, Row{Kind: RowBlank})
			}
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
				rows = append(rows, renderHunkHeaderRow(syntaxName, hunk))
			}
			rows = append(rows, renderHunkRows(syntaxName, hunk, options)...)
		}
	}

	return rows
}

func renderHunkHeaderRow(fileName string, hunk Hunk) Row {
	prefix, code := splitHunkHeader(hunk.Header)
	return Row{
		Kind:     RowHunk,
		Text:     hunk.Header,
		FileName: fileName,
		Prefix:   prefix,
		Code:     code,
	}
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
	marker, code := splitLine(line)
	gutter := renderGutter(line, options, marker)
	return Row{
		Kind:     rowKind(line.Kind),
		Text:     gutter + code,
		FileName: fileName,
		Gutter:   gutter,
		Code:     code,
	}
}

func renderLine(line Line, options RenderOptions) string {
	marker, code := splitLine(line)
	return renderGutter(line, options, marker) + code
}

func renderGutter(line Line, options RenderOptions, marker string) string {
	if !options.ShowLineNumbers {
		return marker
	}

	oldNumber := lineNumber(line.OldLine)
	newNumber := lineNumber(line.NewLine)
	return fmt.Sprintf("%*s %*s %s ", options.LineNumberWidth, oldNumber, options.LineNumberWidth, newNumber, marker)
}

func splitLine(line Line) (marker string, code string) {
	if line.Kind == NoNewline || line.Text == "" {
		return "", line.Text
	}
	return line.Text[:1], line.Text[1:]
}

func splitHunkHeader(header string) (prefix string, code string) {
	const marker = " @@"
	end := strings.Index(header, marker)
	if end == -1 {
		return header, ""
	}
	prefixEnd := end + len(marker)
	if prefixEnd >= len(header) {
		return header, ""
	}
	return header[:prefixEnd], header[prefixEnd:]
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
