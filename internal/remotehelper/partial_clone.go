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
	// Git sets the cloning option only for the initial clone. Even a persisted
	// allowPartialClone setting must never relax a subsequent fetch.
	if h.cloning {
		allowed, err := confluence.ResolveAllowPartialClone(h.remoteName)
		if err != nil {
			return options, err
		}
		options.SkipUnavailableChildren = allowed
	}
	return options, nil
}

func (h *helper) reportWarning(format string, args ...any) {
	if h.err != nil {
		// Missing content must remain visible even with --quiet or no progress.
		fmt.Fprintf(h.err, "confluence: warning: "+format+"\n", args...)
	}
}
