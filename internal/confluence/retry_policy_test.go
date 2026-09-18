package confluence

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestGETDoesNotRetryPermanentErrors(t *testing.T) {
	for _, failure := range []error{
		context.Canceled,
		&net.DNSError{Err: "no such host", Name: "example.test", IsNotFound: true},
		x509.UnknownAuthorityError{},
		errors.New("redirect rejected"),
	} {
		t.Run(failure.Error(), func(t *testing.T) {
			client := NewClient("https://example.test", "token")
			attempts := 0
			client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				attempts++
				return nil, failure
			})
			_, err := client.FetchPage("1")
			if attempts != 1 || !errors.Is(err, failure) {
				t.Fatalf("attempts = %d, error = %v", attempts, err)
			}
		})
	}
}

func TestGETDoesNotRetryHTTPFailures(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := NewClient("https://example.test", "token")
			attempts := 0
			client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("failure"))}, nil
			})
			_, err := client.FetchPage("1")
			var apiErr *APIError
			if attempts != 1 || !errors.As(err, &apiErr) || apiErr.StatusCode != status {
				t.Fatalf("attempts = %d, error = %v", attempts, err)
			}
		})
	}
}

func TestPUTDoesNotRetryConnectionFailures(t *testing.T) {
	client := NewClient("https://example.test", "token")
	attempts := 0
	client.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s", r.Method)
		}
		return nil, proxyConnectionRefused()
	})
	err := client.UpdatePage(PageUpdate{ID: "1", Title: "Page", VersionNumber: 2})
	if attempts != 1 || !errors.Is(err, errConnectionRefused) {
		t.Fatalf("attempts = %d, error = %v", attempts, err)
	}
}

func TestGETCancellationInterruptsBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient("https://example.test", "token")
		attempts := 0
		client.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts++
			return nil, proxyConnectionRefused()
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.BaseURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			time.Sleep(500 * time.Millisecond)
			cancel()
		}()
		start := time.Now()
		_, err = client.doGET(req)
		if attempts != 1 || time.Since(start) != 500*time.Millisecond || !errors.Is(err, context.Canceled) {
			t.Fatalf("attempts = %d, elapsed = %s, error = %v", attempts, time.Since(start), err)
		}
	})
}
