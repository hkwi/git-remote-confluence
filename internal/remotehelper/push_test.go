package remotehelper

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hkwi/git-remote-confluence/internal/confluencetypes"
	"github.com/hkwi/git-remote-confluence/internal/fastimport"
)

func TestHelperPushUpdatesChangedPage(t *testing.T) {
	repo := initPushRepo(t, "<p>吾輩は猫である。</p>", "<p>名前はまだ無い。</p>", 7)
	t.Chdir(repo)

	var putPayload map[string]any
	server := httptest.NewServer(pushMockConfluenceHandler(t, 7, "<p>吾輩は猫である。</p>", &putPayload))
	defer server.Close()
	t.Setenv("CONFLUENCE_PAT", "secret-token")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Main(
		[]string{"origin", "confluence::" + server.URL + "/pages/viewpage.action?pageId=1"},
		strings.NewReader("capabilities\noption progress true\npush HEAD:refs/heads/main\n\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "ok refs/heads/main") {
		t.Fatalf("push status missing:\n%s", stdout.String())
	}
	if putPayload == nil {
		t.Fatal("expected Confluence PUT")
	}
	body := putPayload["body"].(map[string]any)
	storage := body["storage"].(map[string]any)
	if storage["value"] != "<p>名前はまだ無い。</p>" {
		t.Fatalf("storage.value = %v", storage["value"])
	}
	version := putPayload["version"].(map[string]any)
	if version["number"] != float64(8) {
		t.Fatalf("version.number = %v", version["number"])
	}
}

func TestHelperPushRejectsVersionConflict(t *testing.T) {
	repo := initPushRepo(t, "<p>吾輩は猫である。</p>", "<p>名前はまだ無い。</p>", 7)
	t.Chdir(repo)

	var putPayload map[string]any
	server := httptest.NewServer(pushMockConfluenceHandler(t, 8, "<p>吾輩は猫である。</p>", &putPayload))
	defer server.Close()
	t.Setenv("CONFLUENCE_PAT", "secret-token")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Main(
		[]string{"origin", "confluence::" + server.URL + "/pages/viewpage.action?pageId=1"},
		strings.NewReader("capabilities\npush HEAD:refs/heads/main\n\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "error refs/heads/main page 1 version conflict") {
		t.Fatalf("expected version conflict:\n%s", stdout.String())
	}
	if putPayload != nil {
		t.Fatalf("unexpected PUT payload: %#v", putPayload)
	}
}

func initPushRepo(t *testing.T, baseStorage, currentStorage string, version int) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test")

	metadata := fastimport.PageMetadataYAML(
		fastimport.Location{RootType: "page", RootValue: "1"},
		fastimport.PageRecord{
			PageID:     "1",
			Title:      "吾輩は猫である",
			Status:     "current",
			SpaceKey:   "ABC",
			Version:    confluencetypes.Version{Number: version},
			StorageXML: baseStorage,
		},
	)
	writeTestFile(t, repo, "1.md", currentStorage)
	writeTestFile(t, repo, "1.yml", metadata)
	runGit(t, repo, "add", "1.md", "1.yml")
	runGit(t, repo, "commit", "-m", "update page")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func writeTestFile(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pushMockConfluenceHandler(t *testing.T, version int, storageXML string, putPayload *map[string]any) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/rest/api/content/1" {
			http.NotFound(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			writeJSON(t, w, map[string]any{
				"id":     "1",
				"type":   "page",
				"status": "current",
				"title":  "吾輩は猫である",
				"space":  map[string]any{"key": "ABC"},
				"version": map[string]any{
					"number": version,
				},
				"body": map[string]any{"storage": map[string]any{"value": storageXML}},
			})
		case http.MethodPut:
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			*putPayload = payload
			writeJSON(t, w, map[string]any{"id": "1"})
		default:
			http.NotFound(w, r)
		}
	})
}
