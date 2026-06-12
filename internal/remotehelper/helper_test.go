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
)

func TestGitCloneFromMockConfluence(t *testing.T) {
	server := httptest.NewServer(mockConfluenceHandler(t))
	defer server.Close()

	tmp := t.TempDir()
	destination := filepath.Join(tmp, "clone")
	root := repoRoot(t)
	binDir := filepath.Join(tmp, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	helperPath := filepath.Join(binDir, "git-remote-confluence-test")
	build := exec.Command("go", "build", "-o", helperPath, ".")
	build.Dir = root
	build.Env = append(os.Environ(), "GOCACHE="+filepath.Join(tmp, "gocache"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}

	cmd := exec.Command("git", "clone", "--progress", "--verbose", "confluence-test::"+server.URL+"/pages/viewpage.action?pageId=1", destination)
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CONFLUENCE_PAT=secret-token",
		"NO_PROXY=127.0.0.1,localhost",
		"no_proxy=127.0.0.1,localhost",
		"GOCACHE="+filepath.Join(tmp, "gocache"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git clone failed: %v\n%s", err, output)
	}

	rootXML := readCloneFile(t, destination, "1.md")
	if string(rootXML) != "<p>吾輩は猫である。</p>" {
		t.Fatalf("root XML = %q", rootXML)
	}

	childXML := readCloneFile(t, destination, "1", "2.md")
	if string(childXML) != "<p>名前はまだ無い。</p>" {
		t.Fatalf("child XML = %q", childXML)
	}

	metadata := readCloneFile(t, destination, "1.yml")
	if !strings.Contains(string(metadata), `page_id: "1"`) || !strings.Contains(string(metadata), "number: 7") {
		t.Fatalf("unexpected metadata:\n%s", metadata)
	}

	attributes := readCloneFile(t, destination, ".gitattributes")
	if string(attributes) != "*.md filter=confluence-storage diff=markdown\n" {
		t.Fatalf("unexpected attributes:\n%s", attributes)
	}

	checkAttr := exec.Command("git", "-C", destination, "check-attr", "filter", "--", "1.md")
	output, err = checkAttr.CombinedOutput()
	if err != nil {
		t.Fatalf("git check-attr failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "filter: confluence-storage") {
		t.Fatalf("filter attribute missing:\n%s", output)
	}
}

func readCloneFile(t *testing.T, root string, elems ...string) []byte {
	t.Helper()
	path := filepath.Join(append([]string{root}, elems...)...)
	data, err := os.ReadFile(path)
	if err == nil {
		return data
	}

	lsFiles := exec.Command("git", "-C", root, "ls-files", "-s")
	lsOutput, _ := lsFiles.CombinedOutput()
	t.Fatalf("read %s: %v\nworktree:\n%s\ngit index:\n%s", path, err, listWorktree(t, root), lsOutput)
	return nil
}

func listWorktree(t *testing.T, root string) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." || rel == ".git" || strings.HasPrefix(rel, ".git"+string(os.PathSeparator)) {
			if d.IsDir() && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		entries = append(entries, rel)
		return nil
	})
	if err != nil {
		return err.Error()
	}
	return strings.Join(entries, "\n")
}

func TestHelperProgressOutput(t *testing.T) {
	server := httptest.NewServer(mockConfluenceHandler(t))
	defer server.Close()
	t.Setenv("CONFLUENCE_PAT", "secret-token")
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	t.Setenv("no_proxy", "127.0.0.1,localhost")

	input := strings.NewReader(strings.Join([]string{
		"capabilities",
		"option progress true",
		"option verbosity 1",
		"import refs/heads/main",
		"",
	}, "\n"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Main(
		[]string{"origin", "confluence::" + server.URL + "/pages/viewpage.action?pageId=1"},
		input,
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "confluence: fetching page 1") {
		t.Fatalf("stderr progress missing:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "progress confluence: importing page 1") {
		t.Fatalf("fast-import progress missing:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "M 100644 inline 1.md\n") {
		t.Fatalf("fast-import stream missing Markdown page path:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "M 100644 inline 1.xml\n") {
		t.Fatalf("fast-import stream used legacy XML page path:\n%s", stdout.String())
	}
}

func TestHelperResolvesDisplayPageURL(t *testing.T) {
	server := httptest.NewServer(mockConfluenceHandler(t))
	defer server.Close()
	t.Setenv("CONFLUENCE_PAT", "secret-token")
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	t.Setenv("no_proxy", "127.0.0.1,localhost")

	input := strings.NewReader(strings.Join([]string{
		"capabilities",
		"option progress true",
		"option verbosity 1",
		"import refs/heads/main",
		"",
	}, "\n"))
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Main(
		[]string{"origin", "confluence::" + server.URL + "/display/ABC/%E5%90%BE%E8%BC%A9%E3%81%AF%E7%8C%AB%E3%81%A7%E3%81%82%E3%82%8B"},
		input,
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}

	stderrText := stderr.String()
	for _, want := range []string{
		`confluence: resolving page "吾輩は猫である" in space ABC`,
		`confluence: resolved page "吾輩は猫である" in space ABC to page 1`,
		"confluence: root page 1 at " + server.URL,
		"confluence: fetching page 1",
	} {
		if !strings.Contains(stderrText, want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderrText)
		}
	}
	if strings.Contains(stderrText, "fetching space ABC") {
		t.Fatalf("display page URL was treated as a space:\n%s", stderrText)
	}
	if !strings.Contains(stdout.String(), "progress confluence: importing page 1 吾輩は猫である") {
		t.Fatalf("fast-import progress missing resolved page:\n%s", stdout.String())
	}
}

func mockConfluenceHandler(t *testing.T) http.Handler {
	t.Helper()
	pages := map[string]map[string]any{
		"1": {
			"id":     "1",
			"type":   "page",
			"status": "current",
			"title":  "吾輩は猫である",
			"space":  map[string]any{"key": "ABC"},
			"version": map[string]any{
				"number": 7,
				"when":   "2025-01-02T03:04:05.000Z",
				"by":     map[string]any{"displayName": "Author"},
			},
			"body":   map[string]any{"storage": map[string]any{"value": "<p>吾輩は猫である。</p>"}},
			"_links": map[string]any{"webui": "/pages/viewpage.action?pageId=1"},
		},
		"2": {
			"id":        "2",
			"type":      "page",
			"status":    "current",
			"title":     "名前はまだ無い",
			"space":     map[string]any{"key": "ABC"},
			"ancestors": []any{map[string]any{"id": "1"}},
			"version": map[string]any{
				"number": 4,
				"when":   "2025-01-03T03:04:05.000Z",
				"by":     map[string]any{"displayName": "Author"},
			},
			"body":   map[string]any{"storage": map[string]any{"value": "<p>名前はまだ無い。</p>"}},
			"_links": map[string]any{"webui": "/pages/viewpage.action?pageId=2"},
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/rest/api/content/1":
			writeJSON(t, w, pages["1"])
		case "/rest/api/content/1/child/page":
			writeJSON(t, w, map[string]any{"results": []any{pages["2"]}, "size": 1, "limit": 100})
		case "/rest/api/content/2":
			writeJSON(t, w, pages["2"])
		case "/rest/api/content/2/child/page":
			writeJSON(t, w, map[string]any{"results": []any{}, "size": 0, "limit": 100})
		case "/rest/api/content":
			if r.URL.Query().Get("spaceKey") != "ABC" {
				http.NotFound(w, r)
				return
			}
			if title := r.URL.Query().Get("title"); title != "" {
				if title != "吾輩は猫である" {
					writeJSON(t, w, map[string]any{"results": []any{}, "size": 0, "limit": 100})
					return
				}
				writeJSON(t, w, map[string]any{"results": []any{pages["1"]}, "size": 1, "limit": 100})
				return
			}
			writeJSON(t, w, map[string]any{"results": []any{pages["1"], pages["2"]}, "size": 2, "limit": 100})
		default:
			http.NotFound(w, r)
		}
	})
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("go.mod not found")
		}
		wd = parent
	}
}
