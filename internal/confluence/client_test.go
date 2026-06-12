package confluence

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdatePageSendsStoragePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/rest/api/content/1" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["id"] != "1" {
			t.Fatalf("id = %v", payload["id"])
		}
		if payload["title"] != "吾輩は猫である" {
			t.Fatalf("title = %v", payload["title"])
		}
		version := payload["version"].(map[string]any)
		if version["number"] != float64(8) {
			t.Fatalf("version.number = %v", version["number"])
		}
		body := payload["body"].(map[string]any)
		storage := body["storage"].(map[string]any)
		if storage["value"] != "<p>名前はまだ無い。</p>" {
			t.Fatalf("storage.value = %v", storage["value"])
		}
		if storage["representation"] != "storage" {
			t.Fatalf("storage.representation = %v", storage["representation"])
		}
		space := payload["space"].(map[string]any)
		if space["key"] != "ABC" {
			t.Fatalf("space.key = %v", space["key"])
		}

		writeJSON(t, w, Page{ID: "1"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret-token")
	err := client.UpdatePage(PageUpdate{
		ID:            "1",
		Title:         "吾輩は猫である",
		SpaceKey:      "ABC",
		StorageXML:    "<p>名前はまだ無い。</p>",
		VersionNumber: 8,
		Message:       "test",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
