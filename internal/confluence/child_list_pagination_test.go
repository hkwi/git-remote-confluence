package confluence

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestPartialChildListingKeepsCompletedBatches(t *testing.T) {
	client, _ := pageTreeTestClient(t, "", 0, "")
	transport := client.HTTPClient.Transport
	var listed []Page
	for id := 100; id < 200; id++ {
		listed = append(listed, Page{ID: strconv.Itoa(id)})
	}
	var starts []string
	client.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/rest/api/content/2/child/page" {
			start := r.URL.Query().Get("start")
			starts = append(starts, start)
			if start == "0" {
				return childListingResponse(t, listResponse{Results: listed, Links: map[string]string{"next": "?start=100"}}), nil
			}
			if start != "100" {
				t.Fatalf("unexpected pagination offset %s", start)
			}
			return &http.Response{StatusCode: 404, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("not permitted"))}, nil
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/rest/api/content/"), "/")
		id, _ := strconv.Atoi(parts[0])
		if id >= 100 && id < 200 {
			if len(parts) == 1 {
				return childListingResponse(t, Page{ID: parts[0]}), nil
			}
			return childListingResponse(t, listResponse{}), nil
		}
		return transport.RoundTrip(r)
	})
	result, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{SkipUnavailableChildren: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 103 {
		t.Fatalf("imported %d pages, want 103", len(result.Pages))
	}
	if len(result.Pages[1].ChildIDs) != 100 || result.Pages[1].ChildrenErrorStatus != 404 ||
		len(result.UnavailableChildLists) != 1 || strings.Join(starts, ",") != "0,100" {
		t.Fatalf("lost completed batches: pages=%d, parent=%+v, errors=%+v, offsets=%v", len(result.Pages), result.Pages[1], result.UnavailableChildLists, starts)
	}
	for i, child := range result.Pages[2:102] {
		if child.PageID != strconv.Itoa(100+i) || child.ParentID != "2" || child.PathDir != "1/2" {
			t.Fatalf("listed child was not imported: %+v", child)
		}
	}
}

func childListingResponse(t *testing.T, value any) *http.Response {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data)))}
}
