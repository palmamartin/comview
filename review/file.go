package review

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const DefaultFilePath = ".comview/comments.json"

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
