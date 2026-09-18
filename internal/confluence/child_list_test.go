package confluence

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hkwi/git-remote-confluence/internal/fastimport"
	"github.com/hkwi/git-remote-confluence/internal/pagefiles"
)

func TestPartialPageTreeKeepsPageWithUnavailableChildren(t *testing.T) {
	for _, status := range []int{403, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client, requests := pageTreeTestClient(t, "/rest/api/content/2/child/page", status, "not permitted")
			var warnings, progress strings.Builder
			result, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{
				SkipUnavailableChildren: true,
				Warning:                 func(format string, args ...any) { fmt.Fprintf(&warnings, format+"\n", args...) },
				Progress:                func(format string, args ...any) { fmt.Fprintf(&progress, format+"\n", args...) },
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Pages) != 3 || result.Pages[1].PageID != "2" || result.Pages[2].PageID != "3" || len(result.SkippedPages) != 0 {
				t.Fatalf("unexpected partial tree: %+v", result)
			}
			page := result.Pages[1]
			if page.StorageXML != "<p>Known page.</p>" || page.ChildrenErrorStatus != status || len(page.ChildIDs) != 0 ||
				!slices.Equal(result.Pages[0].ChildIDs, []string{"2", "3"}) || len(result.Pages[0].SkippedChildIDs) != 0 {
				t.Fatalf("lost known content or child-list failure: %+v", result.Pages)
			}
			want := []ChildListError{{PageID: "2", ParentID: "1", StatusCode: status}}
			if !reflect.DeepEqual(result.UnavailableChildLists, want) {
				t.Fatalf("child-list errors = %+v", result.UnavailableChildLists)
			}
			if slices.Contains(*requests, "/rest/api/content/4") || strings.Contains(progress.String(), "page 2 has 0 child pages") {
				t.Fatalf("treated unavailable children as a complete list: %v\n%s", *requests, progress.String())
			}
			if !strings.Contains(warnings.String(), fmt.Sprintf("page 2 under parent 1: HTTP %d", status)) {
				t.Fatalf("missing warning: %s", warnings.String())
			}
			metadata := fastimport.PageMetadataYAML(fastimport.Location{RootType: "page", RootValue: "1"}, page)
			if !strings.Contains(metadata, fmt.Sprintf("children_error:\n  http_status: %d\n", status)) {
				t.Fatalf("missing durable error marker:\n%s", metadata)
			}
			if _, err := pagefiles.ParseMetadataYAML([]byte(metadata)); err != nil {
				t.Fatalf("metadata is not readable by the existing parser: %v", err)
			}
		})
	}
}

func TestStrictPageTreeRejectsUnavailableChildLists(t *testing.T) {
	for _, status := range []int{403, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client, _ := pageTreeTestClient(t, "/rest/api/content/2/child/page", status, "not permitted")
			pages, err := FetchPages(client, Location{RootType: "page", RootValue: "1"})
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != status || len(pages) != 0 {
				t.Fatalf("strict fetch = %v, %v", pages, err)
			}
		})
	}
}
