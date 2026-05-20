package review

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const DefaultFilePath = ".comview/comments.json"

const DefaultViewedFilePath = ".comview/viewed.json"

type CommentFile struct {
	Version  int            `json:"version"`
	Source   CommentSource  `json:"source,omitempty"`
	Comments []CommentDraft `json:"comments"`
}

type CommentSource struct {
	Provider   string `json:"provider,omitempty"`
	Owner      string `json:"owner,omitempty"`
	Repo       string `json:"repo,omitempty"`
	PullNumber int    `json:"pull_number,omitempty"`
}

type ViewedFile struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
}

func LoadFile(path string) (CommentFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return CommentFile{Version: 1}, nil
	}
	if err != nil {
		return CommentFile{}, err
	}

	var file CommentFile
	if err := json.Unmarshal(data, &file); err != nil {
		return CommentFile{}, err
	}
	if file.Version == 0 {
		file.Version = 1
	}
	return file, nil
}

func SaveFile(path string, file CommentFile) error {
	if file.Version == 0 {
		file.Version = 1
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func LoadViewedFile(path string) (ViewedFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return newViewedFile(), nil
	}
	if err != nil {
		return ViewedFile{}, err
	}

	var file ViewedFile
	if err := json.Unmarshal(data, &file); err != nil {
		return ViewedFile{}, err
	}
	normalizeViewedFile(&file)
	return file, nil
}

func SaveViewedFile(path string, file ViewedFile) error {
	normalizeViewedFile(&file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (f ViewedFile) IsViewed(path string, hash string) bool {
	if path == "" || hash == "" || f.Files == nil {
		return false
	}
	return f.Files[path] == hash
}

func (f *ViewedFile) Mark(path string, hash string) {
	normalizeViewedFile(f)
	f.Files[path] = hash
}

func (f *ViewedFile) Unmark(path string) {
	if f.Files == nil {
		return
	}
	delete(f.Files, path)
}

func newViewedFile() ViewedFile {
	return ViewedFile{Version: 1, Files: make(map[string]string)}
}

func normalizeViewedFile(file *ViewedFile) {
	if file.Version == 0 {
		file.Version = 1
	}
	if file.Files == nil {
		file.Files = make(map[string]string)
	}
}
