package fastimport

import (
	"testing"

	"github.com/hkwi/git-remote-confluence/internal/confluencetypes"
)

func TestCommentsYAML(t *testing.T) {
	out := CommentsYAML([]CommentRecord{
		{
			ID: "20", Status: "current",
			Version: confluencetypes.Version{
				Number: 3, When: "2025-01-04T03:04:05.000Z",
				By: confluencetypes.User{DisplayName: "Commenter"},
			},
			Body: "<p>コメントです。</p>",
		},
		{
			ID: "21", Status: "current",
			Version:   confluencetypes.Version{Number: 1, By: confluencetypes.User{DisplayName: "Author"}},
			Ancestors: []string{"20"},
			Body:      "<p>reply</p>\n<p>two lines</p>",
		},
	})

	want := `comments:
  - id: "20"
    status: "current"
    version:
      number: 3
      when: "2025-01-04T03:04:05.000Z"
      minor_edit: false
      message: null
      by:
        display_name: "Commenter"
    ancestors:
      []
    body: "<p>コメントです。</p>"
  - id: "21"
    status: "current"
    version:
      number: 1
      when: null
      minor_edit: false
      message: null
      by:
        display_name: "Author"
    ancestors:
      - "20"
    body: "<p>reply</p>\n<p>two lines</p>"
`
	if out != want {
		t.Fatalf("CommentsYAML mismatch:\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
}

func TestEmptyCommentsYAML(t *testing.T) {
	out := CommentsYAML(nil)
	want := "comments:\n  []\n"
	if out != want {
		t.Fatalf("CommentsYAML(nil) = %q, want %q", out, want)
	}
}
