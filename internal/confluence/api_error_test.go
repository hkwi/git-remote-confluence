package confluence

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTPFailuresPreserveDetails(t *testing.T) {
	for _, method := range []string{"page", "update", "attachment"} {
		t.Run(method, func(t *testing.T) {
			client := NewClient("https://example.test", "secret")
			var request *http.Request
			client.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				request = r
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`{"message":"not permitted"}`)),
					Header:     make(http.Header),
				}, nil
			})
			var err error
			switch method {
			case "page":
				_, err = client.FetchPage("2")
			case "update":
				err = client.UpdatePage(PageUpdate{ID: "2", Title: "Page", VersionNumber: 2})
			case "attachment":
				_, err = client.DownloadAttachment(Attachment{ID: "2", Links: map[string]string{"download": "/download/2"}})
			}
			var apiErr *APIError
			if !errors.As(fmt.Errorf("context: %w", err), &apiErr) {
				t.Fatalf("expected APIError, got %v", err)
			}
			if apiErr.StatusCode != 404 || apiErr.Method != request.Method || apiErr.URL != request.URL.String() ||
				apiErr.Body != `{"message":"not permitted"}` {
				t.Fatalf("lost response details: %+v", apiErr)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("error contains bearer token")
			}
		})
	}
}
