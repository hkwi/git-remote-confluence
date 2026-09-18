package confluence

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const maxGETAttempts = 4

// doGET retries transient connection failures before a response is received.
// HTTP responses and failures while reading their bodies retain the caller's
// existing handling. Writes must continue to use HTTPClient.Do directly.
func (c *Client) doGET(req *http.Request) (*http.Response, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	for attempt := 1; ; attempt++ {
		resp, err := client.Do(req)
		if err == nil || resp != nil || req.Method != http.MethodGet || attempt == maxGETAttempts ||
			req.Context().Err() != nil || !retryableConnectionError(err) {
			if err != nil && attempt > 1 {
				err = fmt.Errorf("request failed after %d attempts: %w", attempt, err)
			}
			return resp, err
		}

		delay := time.Second << (attempt - 1)
		report(c.RetryProgress, "retrying GET %s in %s (attempt %d/%d): %v", req.URL.Redacted(), delay, attempt+1, maxGETAttempts, err)
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		}
	}
}

func retryableConnectionError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	// A refused proxy connection is often wrapped in multiple net.OpErrors and
	// is not necessarily classified as Temporary by the network implementation.
	for _, cause := range []error{errConnectionRefused, errConnectionReset, errConnectionAborted, errBrokenPipe, io.EOF, io.ErrUnexpectedEOF} {
		if errors.Is(err, cause) {
			return true
		}
	}
	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
}
