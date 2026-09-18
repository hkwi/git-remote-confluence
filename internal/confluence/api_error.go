package confluence

import "fmt"

// APIError preserves HTTP failure details for callers that need to distinguish
// unavailable content from authentication, rate limiting, and server failures.
type APIError struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Confluence API HTTP %d (%s %s): %s", e.StatusCode, e.Method, e.URL, e.Body)
}
