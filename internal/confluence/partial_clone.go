package confluence

import (
	"fmt"
	"strconv"
)

func ResolveAllowPartialClone(remoteName string) (bool, error) {
	value := resolveRemoteSetting(remoteName, "allowPartialClone",
		"CONFLUENCE_ALLOW_PARTIAL_CLONE", "GIT_REMOTE_CONFLUENCE_ALLOW_PARTIAL_CLONE")
	if value == "" {
		return true, nil
	}
	allowed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid Confluence allowPartialClone value %q: use true or false", value)
	}
	return allowed, nil
}
