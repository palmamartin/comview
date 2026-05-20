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

func TestLoadViewedFileMissingReturnsEmptyFile(t *testing.T) {
	file, err := LoadViewedFile(filepath.Join(t.TempDir(), ".comview", "viewed.json"))
	if err != nil {
		t.Fatal(err)
	}
	if file.Version != 1 || len(file.Files) != 0 {
		t.Fatalf("file = %+v, want empty version 1 file", file)
	}
}

func TestSaveLoadViewedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".comview", "viewed.json")
	file := ViewedFile{Version: 1, Files: map[string]string{"tui/app.go": "89abcde"}}

	if err := SaveViewedFile(path, file); err != nil {
		t.Fatal(err)
	}
	got, err := LoadViewedFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if got.Version != 1 || got.Files["tui/app.go"] != "89abcde" {
		t.Fatalf("file = %+v, want %+v", got, file)
	}
}

func TestViewedFileHashMismatchIsNotViewed(t *testing.T) {
	file := ViewedFile{Version: 1, Files: map[string]string{"tui/app.go": "89abcde"}}
	if file.IsViewed("tui/app.go", "1234567") {
		t.Fatal("hash mismatch reported viewed")
	}
	if !file.IsViewed("tui/app.go", "89abcde") {
		t.Fatal("matching hash not reported viewed")
	}
}
