package review

import "strconv"

type GitHubComment struct {
	ID               int64  `json:"id,omitempty"`
	DiffHunk         string `json:"diff_hunk,omitempty"`
	Path             string `json:"path"`
	Body             string `json:"body"`
	CommitID         string `json:"commit_id,omitempty"`
	OriginalCommitID string `json:"original_commit_id,omitempty"`

	StartLine int  `json:"start_line,omitempty"`
	StartSide Side `json:"start_side,omitempty"`
	Line      int  `json:"line"`
	Side      Side `json:"side"`

	StartColumn *int `json:"start_column,omitempty"`
	EndColumn   *int `json:"end_column,omitempty"`
}

func FromGitHubComment(comment GitHubComment) CommentDraft {
	id := ""
	if comment.ID != 0 {
		id = strconv.FormatInt(comment.ID, 10)
	}
	return CommentDraft{
		ID:               id,
		GitHubID:         comment.ID,
		Path:             comment.Path,
		Body:             comment.Body,
		DiffHunk:         comment.DiffHunk,
		CommitID:         comment.CommitID,
		OriginalCommitID: comment.OriginalCommitID,
		StartLine:        comment.StartLine,
		StartSide:        comment.StartSide,
		Line:             comment.Line,
		Side:             comment.Side,
		StartColumn:      comment.StartColumn,
		EndColumn:        comment.EndColumn,
	}
}

func FromGitHubComments(comments []GitHubComment) []CommentDraft {
	drafts := make([]CommentDraft, 0, len(comments))
	for _, comment := range comments {
		drafts = append(drafts, FromGitHubComment(comment))
	}
	return drafts
}
