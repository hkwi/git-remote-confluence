package remotehelper

import (
	"bytes"
	"testing"
)

func TestPartialCloneRequiresGitCloningOption(t *testing.T) {
	t.Setenv("CONFLUENCE_ALLOW_PARTIAL_CLONE", "true")
	var output bytes.Buffer
	h := &helper{out: &output}
	for _, test := range []struct {
		option string
		allow  bool
	}{
		{"option check-connectivity true", false},
		{"option cloning invalid", false},
		{"option cloning true", true},
		{"option cloning false", false},
	} {
		if err := h.handleOption(test.option); err != nil {
			t.Fatal(err)
		}
		options, err := h.fetchOptions()
		if err != nil || options.SkipUnavailableChildren != test.allow {
			t.Fatalf("%s: allow partial = %v, err = %v", test.option, options.SkipUnavailableChildren, err)
		}
	}
}
