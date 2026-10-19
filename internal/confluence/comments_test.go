package confluence

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hkwi/git-remote-confluence/internal/fastimport"
)

// The shared pageTreeTestClient mock lets page 1 have a two-level thread
// (10 top-level, 11 a reply to 10) and page 3 a three-level thread
// (30 -> 31 -> 32), mirroring the real Confluence /descendant/comment shape.
func TestFetchCommentsParsesBodyAndAncestors(t *testing.T) {
	client, _ := pageTreeTestClient(t, "", 0, "")
	comments, err := client.FetchComments("1")
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 2 {
		t.Fatalf("comments = %d, want 2", len(comments))
	}

	top := comments[0]
	if top.ID != "10" || top.Body.Storage.Value != "<p>Top comment.</p>" {
		t.Fatalf("top comment = %+v", top)
	}
	if len(top.Ancestors) != 0 {
		t.Fatalf("top-level comment ancestors = %+v, want none", top.Ancestors)
	}

	reply := comments[1]
	if reply.ID != "11" || reply.Body.Storage.Value != "<p>Reply to 10.</p>" {
		t.Fatalf("reply comment = %+v", reply)
	}
	if len(reply.Ancestors) != 1 || reply.Ancestors[0].ID != "10" {
		t.Fatalf("reply ancestors = %+v, want [10]", reply.Ancestors)
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

// The import pipeline attaches the whole thread as one CommentRecord per
// comment, with the full reply chain preserved in ancestors.
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
	if len(root.Comments) != 2 {
		t.Fatalf("root comments = %d, want 2", len(root.Comments))
	}
	top := root.Comments[0]
	if top.ID != "10" || top.Body != "<p>Top comment.</p>" || len(top.Ancestors) != 0 {
		t.Fatalf("top record = %+v", top)
	}
	reply := root.Comments[1]
	if reply.ID != "11" || len(reply.Ancestors) != 1 || reply.Ancestors[0] != "10" {
		t.Fatalf("reply record = %+v", reply)
	}
	if root.CommentsErrorStatus != 0 {
		t.Fatalf("unexpected comments_error: %d", root.CommentsErrorStatus)
	}
	// Pages without comments keep an empty thread.
	if len(result.Pages[1].Comments) != 0 {
		t.Fatalf("page 2 should have no comments")
	}
}

// A deep thread on a descendant page must survive intact with the full chain
// (nearest parent first): 30 -> 31 -> 32 on page 3.
func TestPageTreeImportsNestedReplyChain(t *testing.T) {
	client, _ := pageTreeTestClient(t, "", 0, "")
	result, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var deepest fastimport.CommentRecord
	found3 := false
	for _, page := range result.Pages {
		if page.PageID == "3" {
			found3 = true
			if len(page.Comments) != 3 {
				t.Fatalf("page 3 comments = %d, want 3", len(page.Comments))
			}
			deepest = page.Comments[2]
		}
	}
	if !found3 {
		t.Fatalf("page 3 was not imported")
	}
	// 32 replies to 31, which replies to 30 -> chain [31, 30].
	if deepest.ID != "32" {
		t.Fatalf("page3 deepest comment id = %q", deepest.ID)
	}
	want := []string{"31", "30"}
	if len(deepest.Ancestors) != len(want) {
		t.Fatalf("chain = %v, want %v", deepest.Ancestors, want)
	}
	for i := range want {
		if deepest.Ancestors[i] != want[i] {
			t.Fatalf("chain = %v, want %v", deepest.Ancestors, want)
		}
	}
}

func TestPageTreeCommentsUnavailableIsRecorded(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		client, _ := pageTreeTestClient(t, "/rest/api/content/2/descendant/comment", status, "not permitted")
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
