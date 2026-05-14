package review

type Side string

const (
	SideLeft  Side = "LEFT"
	SideRight Side = "RIGHT"
)

type Anchor struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Side Side   `json:"side"`
}

type CommentDraft struct {
	ID               string `json:"id,omitempty"`
	GitHubID         int64  `json:"github_id,omitempty"`
	Path             string `json:"path"`
	Body             string `json:"body"`
	DiffHunk         string `json:"diff_hunk,omitempty"`
	CommitID         string `json:"commit_id,omitempty"`
	OriginalCommitID string `json:"original_commit_id,omitempty"`

	StartLine int  `json:"start_line,omitempty"`
	StartSide Side `json:"start_side,omitempty"`
	Line      int  `json:"line"`
	Side      Side `json:"side"`

	StartColumn *int `json:"start_column,omitempty"`
	EndColumn   *int `json:"end_column,omitempty"`
}
