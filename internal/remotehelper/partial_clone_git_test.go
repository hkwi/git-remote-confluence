package remotehelper

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGitPartialCloneAndStrictFetch(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("CONFLUENCE_PAT", "secret-token")
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	t.Setenv("no_proxy", "127.0.0.1,localhost")
	for _, key := range []string{
		"CONFLUENCE_ALLOW_PARTIAL_CLONE", "GIT_REMOTE_CONFLUENCE_ALLOW_PARTIAL_CLONE",
		"CONFLUENCE_API_ROOT", "GIT_REMOTE_CONFLUENCE_API_ROOT",
		"CONFLUENCE_API_VERSION", "GIT_REMOTE_CONFLUENCE_API_VERSION",
	} {
		t.Setenv(key, "")
	}

	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", filepath.Join(binDir, "git-remote-confluence"), ".")
	build.Dir = repoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, output)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Git searches its exec path before PATH for remote helpers. Isolate it so
	// an installed git-remote-confluence cannot shadow the binary under test.
	t.Setenv("GIT_EXEC_PATH", binDir)

	var unavailable atomic.Int32
	base := mockConfluenceHandler(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unavailable.Load() == 1 && r.URL.Path == "/rest/api/content/3" ||
			unavailable.Load() == 2 && r.URL.Path == "/rest/api/content/3/child/page" {
			http.Error(w, "No parent or not permitted", http.StatusNotFound)
			return
		}
		switch r.URL.Path {
		case "/rest/api/content/1/child/page":
			writeJSON(t, w, map[string]any{"results": []any{map[string]string{"id": "3"}, map[string]string{"id": "2"}}})
		case "/rest/api/content/3":
			writeJSON(t, w, map[string]any{
				"id": "3", "title": "Available before fetch", "version": map[string]int{"number": 1},
				"body": map[string]any{"storage": map[string]string{"value": "<p>Keep this page.</p>"}},
			})
		case "/rest/api/content/3/child/page":
			writeJSON(t, w, map[string]any{"results": []any{}})
		case "/rest/api/content/3/child/attachment":
			writeJSON(t, w, map[string]any{"results": []any{map[string]any{
				"id": "30", "title": "retained.txt", "_links": map[string]string{"download": "/download/30"},
			}}})
		case "/download/30":
			_, _ = w.Write([]byte("retained attachment"))
		default:
			base.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	remoteURL := "confluence::" + server.URL + "/pages/viewpage.action?pageId=1"
	runGit := func(dir string, args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	for index, name := range []string{"content", "children"} {
		t.Run(name, func(t *testing.T) {
			mode := int32(index + 1)
			unavailable.Store(mode)
			if output, err := runGit(tmp, "-c", "confluence.allowPartialClone=false", "clone", remoteURL, filepath.Join(tmp, name+"-strict")); err == nil || !strings.Contains(output, "HTTP 404") {
				t.Fatalf("strict clone should fail: %v\n%s", err, output)
			}

			partial := filepath.Join(tmp, name+"-partial")
			output, err := runGit(tmp, "clone", "--quiet", remoteURL, partial)
			if err != nil {
				t.Fatalf("partial clone: %v\n%s", err, output)
			}
			checkPartialCloneFiles(t, partial, output, name == "children")
			readCloneFile(t, partial, "1", "2.md")
			readCloneFile(t, partial, "1", "attachments", "挿絵.png")
			if output, err := runGit(partial, "rev-parse", "--verify", "refs/heads/main"); err != nil {
				t.Fatalf("partial clone did not create main: %v\n%s", err, output)
			}

			unavailable.Store(0)
			full := filepath.Join(tmp, name+"-full")
			if output, err := runGit(tmp, "clone", remoteURL, full); err != nil {
				t.Fatalf("complete clone: %v\n%s", err, output)
			}
			if metadata := string(readCloneFile(t, full, "1", "3.yml")); strings.Contains(metadata, "children_error:") {
				t.Fatalf("complete listing incorrectly marked as incomplete:\n%s", metadata)
			}
			before, err := runGit(full, "show-ref")
			if err != nil {
				t.Fatal(err)
			}
			if output, err := runGit(full, "config", "confluence.allowPartialClone", "true"); err != nil {
				t.Fatalf("set persistent option: %v\n%s", err, output)
			}
			unavailable.Store(mode)
			if output, err := runGit(full, "fetch", "origin"); err == nil || !strings.Contains(output, "HTTP 404") {
				t.Fatalf("fetch should remain strict: %v\n%s", err, output)
			}
			after, err := runGit(full, "show-ref")
			if err != nil || after != before {
				t.Fatalf("failed fetch changed refs: %v\nbefore:\n%s\nafter:\n%s", err, before, after)
			}
			if page := string(readCloneFile(t, full, "1", "3.md")); page != "<p>Keep this page.</p>" {
				t.Fatalf("failed fetch changed existing page: %q", page)
			}
		})
	}
}

func checkPartialCloneFiles(t *testing.T, destination, output string, incompleteChildren bool) {
	t.Helper()
	rootMetadata := string(readCloneFile(t, destination, "1.yml"))
	if incompleteChildren {
		if !strings.Contains(output, "warning: incomplete child list for page 3 under parent 1: HTTP 404") ||
			!strings.Contains(output, "partial clone: incomplete child lists for 1 pages") {
			t.Fatalf("quiet clone hid incomplete listing:\n%s", output)
		}
		if strings.Contains(rootMetadata, "skipped_children:") || !strings.Contains(rootMetadata, "children:\n  - \"3\"\n  - \"2\"\n") {
			t.Fatalf("readable page was excluded from its parent:\n%s", rootMetadata)
		}
		metadata := string(readCloneFile(t, destination, "1", "3.yml"))
		if !strings.Contains(metadata, "children_error:\n  http_status: 404\n") {
			t.Fatalf("missing incomplete-list marker:\n%s", metadata)
		}
		if page := string(readCloneFile(t, destination, "1", "3.md")); page != "<p>Keep this page.</p>" {
			t.Fatalf("lost readable page: %q", page)
		}
		if attachment := string(readCloneFile(t, destination, "1", "3", "attachments", "retained.txt")); attachment != "retained attachment" {
			t.Fatalf("lost readable attachment: %q", attachment)
		}
		return
	}
	if !strings.Contains(output, "warning: skipping page 3 under parent 1") || !strings.Contains(output, "partial clone: skipped 1") {
		t.Fatalf("quiet clone hid missing content:\n%s", output)
	}
	if !strings.Contains(rootMetadata, "children:\n  - \"2\"\n") || !strings.Contains(rootMetadata, "skipped_children:\n  - \"3\"\n") {
		t.Fatalf("metadata does not distinguish skipped children:\n%s", rootMetadata)
	}
	if _, err := os.Stat(filepath.Join(destination, "1", "3.md")); !os.IsNotExist(err) {
		t.Fatalf("skipped page unexpectedly exists: %v", err)
	}
}
