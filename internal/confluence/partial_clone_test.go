package confluence

import "testing"

func TestResolveAllowPartialClone(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("CONFLUENCE_ALLOW_PARTIAL_CLONE", "")
	t.Setenv("GIT_REMOTE_CONFLUENCE_ALLOW_PARTIAL_CLONE", "")
	if allowed, err := ResolveAllowPartialClone("origin"); !allowed || err != nil {
		t.Fatalf("default = %v, %v", allowed, err)
	}
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "confluence.allowPartialClone")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	t.Setenv("GIT_CONFIG_KEY_1", "remote.origin.allowPartialClone")
	t.Setenv("GIT_CONFIG_VALUE_1", "false")
	if allowed, err := ResolveAllowPartialClone("origin"); allowed || err != nil {
		t.Fatalf("remote setting should take precedence: %v, %v", allowed, err)
	}
	if allowed, err := ResolveAllowPartialClone("other"); !allowed || err != nil {
		t.Fatalf("global fallback = %v, %v", allowed, err)
	}
	t.Setenv("GIT_REMOTE_CONFLUENCE_ALLOW_PARTIAL_CLONE", "true")
	if allowed, err := ResolveAllowPartialClone("origin"); !allowed || err != nil {
		t.Fatalf("environment alias = %v, %v", allowed, err)
	}
	t.Setenv("CONFLUENCE_ALLOW_PARTIAL_CLONE", "false")
	if allowed, err := ResolveAllowPartialClone("origin"); allowed || err != nil {
		t.Fatalf("primary environment setting = %v, %v", allowed, err)
	}
	t.Setenv("CONFLUENCE_ALLOW_PARTIAL_CLONE", "invalid")
	if allowed, err := ResolveAllowPartialClone("origin"); allowed || err == nil {
		t.Fatalf("invalid setting was accepted: %v, %v", allowed, err)
	}
}
