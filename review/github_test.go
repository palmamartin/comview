package review

import "testing"

func TestFromGitHubComment(t *testing.T) {
	start, end := 4, 8
	comment := GitHubComment{
		ID:               42,
		DiffHunk:         "@@ -1 +1 @@",
		Path:             "main.go",
		Body:             "comment",
		CommitID:         "abc123",
		OriginalCommitID: "def456",
		StartLine:        10,
		StartSide:        SideRight,
		Line:             12,
		Side:             SideRight,
		StartColumn:      &start,
		EndColumn:        &end,
	}

	draft := FromGitHubComment(comment)

	if draft.ID != "42" || draft.GitHubID != 42 {
		t.Fatalf("ids = %q/%d, want 42", draft.ID, draft.GitHubID)
	}
	if draft.Path != comment.Path || draft.Body != comment.Body || draft.DiffHunk != comment.DiffHunk {
		t.Fatalf("draft = %+v", draft)
	}
	if draft.CommitID != comment.CommitID || draft.OriginalCommitID != comment.OriginalCommitID {
		t.Fatalf("commit ids = %+v", draft)
	}
	if *draft.StartColumn != start || *draft.EndColumn != end {
		t.Fatalf("columns = %v:%v", draft.StartColumn, draft.EndColumn)
	}
}
