package confluence

import (
	"fmt"
	"net/http"
	"testing"
)

// The shared pageTreeTestClient mock serves one comment on page 1 and none on
// the other pages, so we can verify comment parsing and tree wiring here.
func TestFetchCommentsParsesBodyAndAncestors(t *testing.T) {
	client, _ := pageTreeTestClient(t, "", 0, "")
	comments, err := client.FetchComments("1")
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(comments))
	}
	comment := comments[0]
	if comment.ID != "10" {
		t.Fatalf("comment id = %q", comment.ID)
	}
	if comment.Body.Storage.Value != "<p>Top comment.</p>" {
		t.Fatalf("comment body = %q", comment.Body.Storage.Value)
	}
	// The owning page is present as an ancestor in the API response; the raw
	// parent id must be reported before record conversion drops it.
	if len(comment.Ancestors) != 1 || comment.Ancestors[0].ID != "1" {
		t.Fatalf("comment ancestors = %+v", comment.Ancestors)
	}
}

func TestFetchCommentsEmptyPage(t *testing.T) {
	client, _ := pageTreeTestClient(t, "", 0, "")
	comments, err := client.FetchComments("2")
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0", len(comments))
	}
}

// Verify that the import pipeline attaches the page comment thread as one
// CommentRecord per page.
func TestPageTreeImportsCommentsAsSingleRecord(t *testing.T) {
	client, _ := pageTreeTestClient(t, "", 0, "")
	result, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	root := result.Pages[0]
	if root.PageID != "1" {
		t.Fatalf("root = %q", root.PageID)
	}
	if len(root.Comments) != 1 {
		t.Fatalf("root comments = %d, want 1", len(root.Comments))
	}
	record := root.Comments[0]
	if record.ID != "10" || record.Body != "<p>Top comment.</p>" {
		t.Fatalf("comment record = %+v", record)
	}
	// The owning page itself is dropped from the reply-chain ancestors.
	if len(record.Ancestors) != 0 {
		t.Fatalf("comment ancestors should exclude the owning page: %v", record.Ancestors)
	}
	if root.CommentsErrorStatus != 0 {
		t.Fatalf("unexpected comments_error: %d", root.CommentsErrorStatus)
	}
	// Pages without comments keep an empty thread.
	if len(result.Pages[1].Comments) != 0 {
		t.Fatalf("page 2 should have no comments")
	}
}

func TestPageTreeCommentsUnavailableIsRecorded(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		client, _ := pageTreeTestClient(t, "/rest/api/content/2/child/comment", status, "not permitted")
		var warnings []string
		result, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{
			Warning: func(format string, args ...any) {
				warnings = append(warnings, fmt.Sprintf(format, args...))
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, page := range result.Pages {
			if page.PageID != "2" {
				continue
			}
			found = true
			if page.CommentsErrorStatus != status {
				t.Fatalf("page 2 comments_error = %d, want %d (HTTP %d)", page.CommentsErrorStatus, status, status)
			}
		}
		if !found {
			t.Fatalf("page 2 was not imported")
		}
		if len(result.UnavailableComments) != 1 || result.UnavailableComments[0].PageID != "2" ||
			result.UnavailableComments[0].StatusCode != status {
			t.Fatalf("UnavailableComments = %+v", result.UnavailableComments)
		}
		if len(warnings) == 0 {
			t.Fatalf("expected a warning")
		}
	}
}
