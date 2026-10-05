package confluence

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hkwi/git-remote-confluence/internal/fastimport"
	"github.com/hkwi/git-remote-confluence/internal/pagefiles"
)

func TestUnavailableAttachmentListKeepsPage(t *testing.T) {
	for _, status := range []int{403, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client, _ := pageTreeTestClient(t, "/rest/api/content/2/child/attachment", status,
				`{"message":"User not permitted to view attachments on content"}`)
			var warnings strings.Builder
			result, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{
				Warning: func(format string, args ...any) { fmt.Fprintf(&warnings, format+"\n", args...) },
			})
			if err != nil {
				t.Fatalf("attachment permissions aborted the import: %v", err)
			}
			if len(result.Pages) != 4 {
				t.Fatalf("unexpected pages: %+v", result.Pages)
			}
			page := pageByID(t, result.Pages, "2")
			if len(page.Attachments) != 0 || page.AttachmentsErrorStatus != status {
				t.Fatalf("page 2 = %+v", page)
			}
			want := []AttachmentError{{PageID: "2", StatusCode: status}}
			if !reflect.DeepEqual(result.UnavailableAttachments, want) {
				t.Fatalf("attachment errors = %+v", result.UnavailableAttachments)
			}
			if !strings.Contains(warnings.String(), fmt.Sprintf("attachment list unavailable for page 2: HTTP %d", status)) {
				t.Fatalf("missing warning: %s", warnings.String())
			}
			metadata := fastimport.PageMetadataYAML(fastimport.Location{RootType: "page", RootValue: "1"}, page)
			if !strings.Contains(metadata, fmt.Sprintf("attachments_error:\n  http_status: %d\n", status)) {
				t.Fatalf("missing durable error marker:\n%s", metadata)
			}
			if _, err := pagefiles.ParseMetadataYAML([]byte(metadata)); err != nil {
				t.Fatalf("metadata is not readable by the existing parser: %v", err)
			}
		})
	}
}

func TestUnavailableAttachmentDownloadKeepsOtherContent(t *testing.T) {
	client, _ := pageTreeTestClient(t, "/download/8", 403, "forbidden")
	var warnings strings.Builder
	result, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{
		Warning: func(format string, args ...any) { fmt.Fprintf(&warnings, format+"\n", args...) },
	})
	if err != nil {
		t.Fatalf("attachment download permissions aborted the import: %v", err)
	}
	page := pageByID(t, result.Pages, "3")
	if len(page.Attachments) != 0 || page.AttachmentsErrorStatus != 403 {
		t.Fatalf("page 3 = %+v", page)
	}
	if !strings.Contains(warnings.String(), `skipping attachment 8 ("file.txt") on page 3: HTTP 403`) {
		t.Fatalf("missing warning: %s", warnings.String())
	}
}

func pageByID(t *testing.T, pages []fastimport.PageRecord, pageID string) fastimport.PageRecord {
	t.Helper()
	for _, page := range pages {
		if page.PageID == pageID {
			return page
		}
	}
	t.Fatalf("page %s is missing from %+v", pageID, pages)
	return fastimport.PageRecord{}
}

func TestUnexpectedAttachmentFailureStillFails(t *testing.T) {
	client, _ := pageTreeTestClient(t, "/rest/api/content/2/child/attachment", 500, "boom")
	_, err := FetchPages(client, Location{RootType: "page", RootValue: "1"})
	if err == nil || !strings.Contains(err.Error(), "fetch attachments for page 2") {
		t.Fatalf("server failure = %v", err)
	}
}
