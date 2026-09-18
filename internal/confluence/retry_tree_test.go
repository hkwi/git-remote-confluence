package confluence

import (
	"errors"
	"net/http"
	"testing"
	"testing/synctest"
)

func TestPartialPageTreeRejectsPersistentConnectionFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client, _ := pageTreeTestClient(t, "", 0, "")
		transport := client.HTTPClient.Transport
		attempts := 0
		client.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/rest/api/content/2/child/page" {
				attempts++
				return nil, proxyConnectionRefused()
			}
			return transport.RoundTrip(r)
		})
		result, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{SkipUnavailableChildren: true})
		if attempts != 4 || !errors.Is(err, errConnectionRefused) || len(result.Pages) != 0 || len(result.SkippedPages) != 0 {
			t.Fatalf("attempts = %d, result = %+v, error = %v", attempts, result, err)
		}
	})
}
