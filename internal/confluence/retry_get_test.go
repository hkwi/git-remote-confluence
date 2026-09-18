package confluence

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func proxyConnectionRefused() error {
	return &net.OpError{Op: "proxyconnect", Net: "tcp", Err: &net.OpError{
		Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 8080},
		Err: &os.SyscallError{Syscall: "connect", Err: errConnectionRefused},
	}}
}

func TestGETRetriesProxyConnectionRefused(t *testing.T) {
	for _, test := range []struct {
		name, body string
		fetch      func(*Client) error
	}{
		{"page", `{"id":"1"}`, func(c *Client) error { _, err := c.FetchPage("1"); return err }},
		{"children", `{"results":[{"id":"2"}]}`, func(c *Client) error { _, err := c.FetchChildren("1"); return err }},
		{"attachment list", `{"results":[]}`, func(c *Client) error { _, err := c.FetchAttachments("1"); return err }},
		{"download", "file contents", func(c *Client) error {
			data, err := c.DownloadAttachment(Attachment{ID: "2", Links: map[string]string{"download": "/download/2"}})
			if err == nil && string(data) != "file contents" {
				return fmt.Errorf("download = %q", data)
			}
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				client := NewClient("https://example.test", "token")
				var attempts int
				var firstURL string
				var warnings []string
				client.RetryProgress = func(format string, args ...any) {
					warnings = append(warnings, fmt.Sprintf(format, args...))
				}
				client.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
					attempts++
					if attempts == 1 {
						firstURL = r.URL.String()
					}
					if r.Method != http.MethodGet || r.URL.String() != firstURL || r.Header.Get("Authorization") != "Bearer token" {
						t.Fatal("retry changed the method, URL, or authorization")
					}
					if attempts < 3 {
						return nil, proxyConnectionRefused()
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(test.body))}, nil
				})
				start := time.Now()
				if err := test.fetch(client); err != nil {
					t.Fatal(err)
				}
				if attempts != 3 || time.Since(start) != 3*time.Second || len(warnings) != 2 {
					t.Fatalf("attempts = %d, elapsed = %s, warnings = %v", attempts, time.Since(start), warnings)
				}
			})
		})
	}
}

func TestGETRetryExhaustionPreservesCause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient("https://example.test", "token")
		attempts := 0
		client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts++
			return nil, proxyConnectionRefused()
		})
		start := time.Now()
		_, err := client.FetchChildren("1")
		if attempts != 4 || time.Since(start) != 7*time.Second || !errors.Is(err, errConnectionRefused) ||
			!strings.Contains(err.Error(), "after 4 attempts") {
			t.Fatalf("attempts = %d, elapsed = %s, error = %v", attempts, time.Since(start), err)
		}
	})
}

func TestPageTreeRetriesOnlyTheFailedChildListing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client, requests := pageTreeTestClient(t, "", 0, "")
		transport := client.HTTPClient.Transport
		attempts := 0
		client.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/rest/api/content/2/child/page" {
				attempts++
				if attempts < 3 {
					return nil, proxyConnectionRefused()
				}
			}
			return transport.RoundTrip(r)
		})
		result, err := FetchPagesWithOptions(client, Location{RootType: "page", RootValue: "1"}, FetchOptions{SkipUnavailableChildren: true})
		if err != nil || len(result.Pages) != 4 || len(result.SkippedPages) != 0 || attempts != 3 {
			t.Fatalf("attempts = %d, result = %+v, error = %v", attempts, result, err)
		}
		seen := map[string]bool{}
		for _, path := range *requests {
			if seen[path] {
				t.Fatalf("restarted completed request: %s", path)
			}
			seen[path] = true
		}
	})
}
