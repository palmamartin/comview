package diff

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/rockorager/comview/review"
)

type Document struct {
	Preamble []string
	Files    []File
	Metadata Metadata
}

type Metadata struct {
	SourceKind string
	CommitID   string
}

type File struct {
	Header  []string
	OldName string
	NewName string
	Hunks   []Hunk
}

type Hunk struct {
	Header   string
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []Line
}

type Line struct {
	Kind    LineKind
	OldLine int
	NewLine int
	Text    string
}

type LineKind int

const (
	Context LineKind = iota
	Add
	Delete
	NoNewline
)

type Row struct {
	Kind        RowKind
	Text        string
	FileName    string
	Review      review.Anchor
	Gutter      string
	Marker      string
	Code        string
	Prefix      string
	InlineSpans []InlineSpan
}

type InlineSpan struct {
	Start int
	End   int
	Kind  InlineKind
}

type InlineKind int

const (
	InlineChange InlineKind = iota
)

type RowKind int

const (
	RowPreamble RowKind = iota
	RowCommitHeader
	RowCommitMeta
	RowCommitMessage
	RowCommitTrailer
	RowBlank
	RowFile
	RowMeta
	RowHunk
	RowContext
	RowAdd
	RowDelete
	RowNoNewline
)

var hunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

func Parse(input string) (Document, error) {
	var doc Document
	var currentFile *File
	var currentHunk *Hunk
	oldLine := 0
	newLine := 0

	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Buffer(make([]byte, 1024), 1024*1024*8)

	for scanner.Scan() {
		line := scanner.Text()
		if currentFile == nil {
			parseDocumentMetadata(&doc, line)
		}

		if strings.HasPrefix(line, "diff --git ") {
			doc.Files = append(doc.Files, File{
				Header: []string{line},
			})
			currentFile = &doc.Files[len(doc.Files)-1]
			currentHunk = nil
			continue
		}

		if currentFile == nil {
			doc.Preamble = append(doc.Preamble, line)
			continue
		}

		if hunkHeader.MatchString(line) {
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return Document{}, err
			}
			currentFile.Hunks = append(currentFile.Hunks, hunk)
			currentHunk = &currentFile.Hunks[len(currentFile.Hunks)-1]
			oldLine = currentHunk.OldStart
			newLine = currentHunk.NewStart
			continue
		}

		if currentHunk == nil {
			currentFile.Header = append(currentFile.Header, line)
			if strings.HasPrefix(line, "--- ") {
				currentFile.OldName = strings.TrimPrefix(line, "--- ")
			}
			if strings.HasPrefix(line, "+++ ") {
				currentFile.NewName = strings.TrimPrefix(line, "+++ ")
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "+"):
			currentHunk.Lines = append(currentHunk.Lines, Line{
				Kind:    Add,
				NewLine: newLine,
				Text:    line,
			})
			newLine++
		case strings.HasPrefix(line, "-"):
			currentHunk.Lines = append(currentHunk.Lines, Line{
				Kind:    Delete,
				OldLine: oldLine,
				Text:    line,
			})
			oldLine++
		case strings.HasPrefix(line, `\`):
			currentHunk.Lines = append(currentHunk.Lines, Line{
				Kind: NoNewline,
				Text: line,
			})
		default:
			currentHunk.Lines = append(currentHunk.Lines, Line{
				Kind:    Context,
				OldLine: oldLine,
				NewLine: newLine,
				Text:    line,
			})
			oldLine++
			newLine++
		}
	}

	if err := scanner.Err(); err != nil {
		return Document{}, err
	}

	return doc, nil
}

func parseDocumentMetadata(doc *Document, line string) {
	if doc.Metadata.CommitID != "" {
		return
	}
	if strings.HasPrefix(line, "commit ") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			doc.Metadata.SourceKind = "show"
			doc.Metadata.CommitID = fields[1]
		}
	}
}

func parseHunkHeader(line string) (Hunk, error) {
	matches := hunkHeader.FindStringSubmatch(line)
	if matches == nil {
		return Hunk{}, fmt.Errorf("invalid hunk header: %q", line)
	}

	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return Hunk{}, err
	}
	oldCount, err := parseCount(matches[2])
	if err != nil {
		return Hunk{}, err
	}
	newStart, err := strconv.Atoi(matches[3])
	if err != nil {
		return Hunk{}, err
	}
	newCount, err := parseCount(matches[4])
	if err != nil {
		return Hunk{}, err
	}

	return Hunk{
		Header:   line,
		OldStart: oldStart,
		OldCount: oldCount,
		NewStart: newStart,
		NewCount: newCount,
	}, nil
}

func parseCount(value string) (int, error) {
	if value == "" {
		return 1, nil
	}
	return strconv.Atoi(value)
}

func rowKind(kind LineKind) RowKind {
	switch kind {
	case Add:
		return RowAdd
	case Delete:
		return RowDelete
	case NoNewline:
		return RowNoNewline
	default:
		return RowContext
	}
}

func fileName(file File) string {
	oldName := displayFileName(file.OldName)
	newName := displayFileName(file.NewName)
	switch {
	case oldName != "" && newName != "" && oldName != newName && oldName != "/dev/null" && newName != "/dev/null":
		return oldName + " -> " + newName
	case newName != "" && newName != "/dev/null":
		return newName
	case oldName != "" && oldName != "/dev/null":
		return oldName
	case len(file.Header) > 0:
		return file.Header[0]
	default:
		return "(unknown file)"
	}
}

func displayFileName(name string) string {
	if name == "" || name == "/dev/null" {
		return name
	}
	return stripDiffPathPrefix(name)
}

func syntaxFileName(file File) string {
	switch {
	case file.NewName != "" && file.NewName != "/dev/null":
		return stripDiffPathPrefix(file.NewName)
	case file.OldName != "" && file.OldName != "/dev/null":
		return stripDiffPathPrefix(file.OldName)
	default:
		return fileName(file)
	}
}

func stripDiffPathPrefix(name string) string {
	switch {
	case strings.HasPrefix(name, "a/"), strings.HasPrefix(name, "b/"):
		return name[2:]
	default:
		return name
	}
}
