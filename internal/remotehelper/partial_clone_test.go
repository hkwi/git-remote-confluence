package remotehelper

import (
	"bytes"
	"testing"
)

func TestPartialImportAppliesToFetchAsWellAsClone(t *testing.T) {
	var output bytes.Buffer
	for _, test := range []struct {
		setting string
		option  string
		allow   bool
	}{
		{"true", "option cloning true", true},
		{"true", "option cloning false", true},
		{"false", "option cloning true", false},
		{"false", "option cloning false", false},
	} {
		t.Setenv("CONFLUENCE_ALLOW_PARTIAL_CLONE", test.setting)
		h := &helper{out: &output}
		if err := h.handleOption(test.option); err != nil {
			t.Fatal(err)
		}
		options, err := h.fetchOptions()
		if err != nil || options.SkipUnavailableChildren != test.allow {
			t.Fatalf("setting %q with %s: allow partial = %v, err = %v",
				test.setting, test.option, options.SkipUnavailableChildren, err)
		}
	}
}
