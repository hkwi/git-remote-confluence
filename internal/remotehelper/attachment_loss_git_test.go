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

func TestGitFetchWarnsWhenAttachmentPermissionIsRevoked(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("CONFLUENCE_PAT", "secret-token")
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	t.Setenv("no_proxy", "127.0.0.1,localhost")

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
	t.Setenv("GIT_EXEC_PATH", binDir)

	var forbidden atomic.Bool
	base := mockConfluenceHandler(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if forbidden.Load() && r.URL.Path == "/rest/api/content/1/child/attachment" {
			http.Error(w, "User not permitted to view attachments on content", http.StatusForbidden)
			return
		}
		base.ServeHTTP(w, r)
	}))
	defer server.Close()

	runGit := func(dir string, args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	clone := filepath.Join(tmp, "clone")
	remoteURL := "confluence::" + server.URL + "/pages/viewpage.action?pageId=1"
	if output, err := runGit(tmp, "clone", remoteURL, clone); err != nil {
		t.Fatalf("clone: %v\n%s", err, output)
	}
	const attachmentPath = "1/attachments/挿絵.png"
	readCloneFile(t, clone, "1", "attachments", "挿絵.png")

	forbidden.Store(true)
	output, err := runGit(clone, "fetch", "--quiet", "origin")
	if err != nil {
		t.Fatalf("revoked attachment permission failed the fetch: %v\n%s", err, output)
	}
	if !strings.Contains(output, "page 1: HTTP 403 dropped 1 attachment(s) kept by the previous import: "+attachmentPath) {
		t.Fatalf("quiet fetch hid the dropped attachment:\n%s", output)
	}

	fetched, err := runGit(clone, "ls-tree", "-r", "-z", "--name-only", "FETCH_HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fetched, attachmentPath) {
		t.Fatalf("forbidden attachment was imported again:\n%s", fetched)
	}
	previous, err := runGit(clone, "ls-tree", "-r", "-z", "--name-only", "FETCH_HEAD^")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(previous, attachmentPath) {
		t.Fatalf("earlier commit no longer retains the attachment:\n%s", previous)
	}
	if metadata, err := runGit(clone, "show", "FETCH_HEAD:1.yml"); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(metadata, "attachments_error:\n  http_status: 403\n") {
		t.Fatalf("missing durable error marker:\n%s", metadata)
	}
}
