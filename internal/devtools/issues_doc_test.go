package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssuesDocHasReviewFollowupNotes(t *testing.T) {
	root := findRepoRoot(t)

	issuesPath := filepath.Join(root, "docs", "reviews", "issues-2026-01-28.md")
	issuesBytes, err := os.ReadFile(issuesPath)
	if err != nil {
		t.Fatalf("read %q: %v", issuesPath, err)
	}

	issues := string(issuesBytes)
	wantSubstrings := []string{
		"## 更新 (2026-01-28)",
		"Issue #6/#7/#9 は対応不要",
		"`RequeueAfter` + error",
		"controller-runtime",
		"RequeueAfter は無視される",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(issues, want) {
			t.Errorf("docs/reviews/issues-2026-01-28.md missing %q", want)
		}
	}
}
