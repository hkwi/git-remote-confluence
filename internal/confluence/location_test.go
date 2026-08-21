package confluence

import (
	"os/exec"
	"testing"
)

func TestParseLocationPageURL(t *testing.T) {
	location, err := ParseLocation("confluence::https://cf.example.test/wiki/pages/viewpage.action?pageId=123456789")
	if err != nil {
		t.Fatal(err)
	}
	if location.BaseURL != "https://cf.example.test/wiki" {
		t.Fatalf("base URL = %q", location.BaseURL)
	}
	if location.RootType != "page" || location.RootValue != "123456789" {
		t.Fatalf("root = %s %s", location.RootType, location.RootValue)
	}
}

func TestParseLocationSpaceURL(t *testing.T) {
	location, err := ParseLocation("confluence:https://cf.example.test/display/ABC")
	if err != nil {
		t.Fatal(err)
	}
	if location.BaseURL != "https://cf.example.test" {
		t.Fatalf("base URL = %q", location.BaseURL)
	}
	if location.RootType != "space" || location.RootValue != "ABC" {
		t.Fatalf("root = %s %s", location.RootType, location.RootValue)
	}
}

func TestParseLocationDisplayPageURL(t *testing.T) {
	location, err := ParseLocation("confluence:https://cf.example.test/display/ABC/%E5%90%BE%E8%BC%A9%E3%81%AF%E7%8C%AB%E3%81%A7%E3%81%82%E3%82%8B")
	if err != nil {
		t.Fatal(err)
	}
	if location.BaseURL != "https://cf.example.test" {
		t.Fatalf("base URL = %q", location.BaseURL)
	}
	if location.RootType != "page" || location.RootValue != "" {
		t.Fatalf("root = %s %s", location.RootType, location.RootValue)
	}
	if location.SpaceKey != "ABC" || location.PageTitle != "吾輩は猫である" {
		t.Fatalf("page ref = space %q title %q", location.SpaceKey, location.PageTitle)
	}
}

func TestParseLocationDisplayPageURLPreservesEscapedPlus(t *testing.T) {
	location, err := ParseLocation("confluence:https://cf.example.test/display/ABC/C%2B%2B")
	if err != nil {
		t.Fatal(err)
	}
	if location.PageTitle != "C++" {
		t.Fatalf("page title = %q", location.PageTitle)
	}
}

func TestResolveAPIPathFromEnvironment(t *testing.T) {
	t.Setenv("CONFLUENCE_API_ROOT", "/custom/api/root/")
	t.Setenv("CONFLUENCE_API_VERSION", "/2.0/")

	apiRoot, apiVersion := ResolveAPIPath("origin")
	if apiRoot != "custom/api/root" || apiVersion != "2.0" {
		t.Fatalf("API path = %q version %q", apiRoot, apiVersion)
	}
}

func TestResolveAPIPathDefaultsToUnversionedRESTAPI(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("CONFLUENCE_API_ROOT", "")
	t.Setenv("GIT_REMOTE_CONFLUENCE_API_ROOT", "")
	t.Setenv("CONFLUENCE_API_VERSION", "")
	t.Setenv("GIT_REMOTE_CONFLUENCE_API_VERSION", "")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	apiRoot, apiVersion := ResolveAPIPath("invalid remote name")
	if apiRoot != "rest/api" || apiVersion != "" {
		t.Fatalf("API path = %q version %q", apiRoot, apiVersion)
	}
}

func TestResolveAPIPathFromRemoteConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	for _, args := range [][]string{
		{"init"},
		{"config", "remote.origin.apiRoot", "legacy/rest/api"},
		{"config", "remote.origin.apiVersion", "1.0"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}

	apiRoot, apiVersion := ResolveAPIPath("origin")
	if apiRoot != "legacy/rest/api" || apiVersion != "1.0" {
		t.Fatalf("API path = %q version %q", apiRoot, apiVersion)
	}
}
