package remotehelper

import (
	"fmt"

	"github.com/hkwi/git-remote-confluence/internal/confluence"
)

func (h *helper) fetchOptions() (confluence.FetchOptions, error) {
	options := confluence.FetchOptions{
		Progress: h.reportProgress,
		Warning:  h.reportWarning,
	}
	// Fetches honor the setting too: content that became unavailable after the
	// clone would otherwise fail every later fetch permanently. Pages dropped
	// relative to the previous import are reported instead.
	allowed, err := confluence.ResolveAllowPartialClone(h.remoteName)
	if err != nil {
		return options, err
	}
	options.SkipUnavailableChildren = allowed
	return options, nil
}

func (h *helper) reportWarning(format string, args ...any) {
	if h.err != nil {
		// Missing content must remain visible even with --quiet or no progress.
		fmt.Fprintf(h.err, "confluence: warning: "+format+"\n", args...)
	}
}
