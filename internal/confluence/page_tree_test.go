package confluence

import (
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestPartialPageTreeSkipsOnlyUnavailableSubtrees(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client, requests := pageTreeTestClient(t, "/rest/api/content/2", status, "unavailable")
			var warnings []string
			result, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{
				SkipUnavailableChildren: true,
				Warning: func(format string, args ...any) {
					warnings = append(warnings, fmt.Sprintf(format, args...))
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Pages) != 2 || result.Pages[0].PageID != "1" || result.Pages[1].PageID != "3" {
				t.Fatalf("unexpected imported pages: %+v", result.Pages)
			}
			root, sibling := result.Pages[0], result.Pages[1]
			if !slices.Equal(root.ChildIDs, []string{"3"}) || !slices.Equal(root.SkippedChildIDs, []string{"2"}) {
				t.Fatalf("children = %v, skipped = %v", root.ChildIDs, root.SkippedChildIDs)
			}
			wantSkipped := []SkippedPage{{PageID: "2", ParentID: "1", StatusCode: status}}
			if !reflect.DeepEqual(result.SkippedPages, wantSkipped) {
				t.Fatalf("skipped = %+v", result.SkippedPages)
			}
			if sibling.ParentID != "1" || sibling.ContentPath() != "1/3.md" || len(sibling.Attachments) != 1 {
				t.Fatalf("sibling was not fully imported: %+v", sibling)
			}
			for _, path := range *requests {
				if strings.HasPrefix(path, "/rest/api/content/2/") || path == "/rest/api/content/4" {
					t.Fatalf("requested skipped subtree: %s", path)
				}
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], fmt.Sprintf("page 2 under parent 1 and its subtree: HTTP %d", status)) {
				t.Fatalf("missing skip warning: %v", warnings)
			}
		})
	}
}

// The listed child 2 has a descendant, while sibling 3 has an attachment.
// This lets tests verify the boundary between skipping a subtree and completing
// the rest of the import without a live Confluence service.
func pageTreeTestClient(t *testing.T, failurePath string, status int, body string) (*Client, *[]string) {
	t.Helper()
	responses := map[string]string{
		"/rest/api/content/1":                  `{"id":"1"}`,
		"/rest/api/content/1/child/page":       `{"results":[{"id":"2"},{"id":"3"}]}`,
		"/rest/api/content/1/child/attachment": `{"results":[]}`,
		"/rest/api/content/2":                  `{"id":"2","version":{"number":1},"body":{"storage":{"value":"<p>Known page.</p>"}}}`,
		"/rest/api/content/2/child/page":       `{"results":[{"id":"4"}]}`,
		"/rest/api/content/2/child/attachment": `{"results":[]}`,
		"/rest/api/content/3":                  `{"id":"3"}`,
		"/rest/api/content/3/child/page":       `{"results":[]}`,
		"/rest/api/content/3/child/attachment": `{"results":[{"id":"8","title":"file.txt","_links":{"download":"/download/8"}}]}`,
		"/rest/api/content/4":                  `{"id":"4"}`,
		"/rest/api/content/4/child/page":       `{"results":[]}`,
		"/rest/api/content/4/child/attachment": `{"results":[]}`,
		"/download/8":                          "attachment bytes",
	}
	client := NewClient("https://example.test", "token")
	var requests []string
	client.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		path := r.URL.Path
		requests = append(requests, path)
		response, ok := responses[path]
		code := http.StatusOK
		if path == failurePath {
			code, response, ok = status, body, true
		}
		if !ok {
			t.Fatalf("unexpected request: %s", path)
		}
		return &http.Response{
			StatusCode: code, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(response)),
		}, nil
	})
	return client, &requests
}
