package confluence

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestPartialPageTreeStillFailsOnOtherHTTPErrors(t *testing.T) {
	for _, test := range []struct {
		name, path string
		status     int
	}{
		{"root forbidden", "/rest/api/content/1", 403},
		{"root missing", "/rest/api/content/1", 404},
		{"child unauthorized", "/rest/api/content/2", 401},
		{"child rate limited", "/rest/api/content/2", 429},
		{"child server error", "/rest/api/content/2", 500},
		{"child unavailable", "/rest/api/content/2", 503},
		{"child list forbidden", "/rest/api/content/2/child/page", 403},
		{"child list missing", "/rest/api/content/2/child/page", 404},
		{"attachment list forbidden", "/rest/api/content/2/child/attachment", 403},
		{"attachment list missing", "/rest/api/content/2/child/attachment", 404},
		{"download forbidden", "/download/8", 403},
		{"download missing", "/download/8", 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, _ := pageTreeTestClient(t, test.path, test.status, "failure")
			result, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{SkipUnavailableChildren: true})
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != test.status {
				t.Fatalf("expected HTTP %d, got %v", test.status, err)
			}
			if len(result.Pages) != 0 {
				t.Fatal("failed traversal returned an importable partial snapshot")
			}
		})
	}
}

func TestStrictPageTreeRejectsUnavailableChildren(t *testing.T) {
	for _, status := range []int{403, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client, _ := pageTreeTestClient(t, "/rest/api/content/2", status, "failure")
			pages, err := FetchPages(client, Location{RootType: "page", RootValue: "1"})
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != status || len(pages) != 0 {
				t.Fatalf("strict fetch = %v, %v", pages, err)
			}
		})
	}
}

func TestPartialPageTreeRejectsInvalidResponses(t *testing.T) {
	for _, body := range []string{"not JSON", `{}`} {
		t.Run(body, func(t *testing.T) {
			client, _ := pageTreeTestClient(t, "/rest/api/content/2", 200, body)
			_, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{SkipUnavailableChildren: true})
			if err == nil {
				t.Fatal("invalid page response was ignored")
			}
		})
	}
}

func TestPartialPageTreeRejectsTransportErrors(t *testing.T) {
	for _, failure := range []error{context.DeadlineExceeded, errors.New("Confluence API HTTP 404: network failed")} {
		t.Run(failure.Error(), func(t *testing.T) {
			client, _ := pageTreeTestClient(t, "", 0, "")
			transport := client.HTTPClient.Transport
			client.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/rest/api/content/2" {
					return nil, failure
				}
				return transport.RoundTrip(r)
			})
			_, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{SkipUnavailableChildren: true})
			if !errors.Is(err, failure) {
				t.Fatalf("transport failure was not preserved: %v", err)
			}
		})
	}
}
