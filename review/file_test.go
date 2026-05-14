package review

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".comview", "comments.json")
	start, end := 3, 7
	file := CommentFile{
		Version: 1,
		Source: CommentSource{
			Provider:   "github",
			Owner:      "rockorager",
			Repo:       "comview",
			PullNumber: 12,
		},
		Comments: []CommentDraft{{
			GitHubID:    123,
			Path:        "tui/app.go",
			Body:        "comment",
			Line:        10,
			Side:        SideRight,
			StartColumn: &start,
			EndColumn:   &end,
		}},
	}

	if err := SaveFile(path, file); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if got.Version != file.Version || got.Source != file.Source || len(got.Comments) != 1 {
		t.Fatalf("file = %+v, want %+v", got, file)
	}
	if got.Comments[0].GitHubID != 123 || *got.Comments[0].StartColumn != start || *got.Comments[0].EndColumn != end {
		t.Fatalf("comment = %+v", got.Comments[0])
	}
}

func TestLoadFileMissingReturnsEmptyFile(t *testing.T) {
	file, err := LoadFile(filepath.Join(t.TempDir(), ".comview", "comments.json"))
	if err != nil {
		t.Fatal(err)
	}
	if file.Version != 1 || len(file.Comments) != 0 {
		t.Fatalf("file = %+v, want empty version 1 file", file)
	}
}
