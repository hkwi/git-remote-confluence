package confluence

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAttachmentPointerUsesStableMetadata(t *testing.T) {
	pointer, err := attachmentPointer("https://cf.example.test/wiki", "123", Attachment{
		ID:         "456",
		Title:      "diagram one.png",
		Version:    Version{Number: 7},
		Extensions: AttachmentExtensions{FileSize: 42, MediaType: "image/png"},
		Links: map[string]string{
			"download": "/wiki/download/attachments/123/diagram%20one.png?version=7&modificationDate=99&api=v2",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(pointer)
	for _, want := range []string{
		"attachment-id 456\n",
		"attachment-version 7\n",
		"filename diagram one.png\n",
		"size 42\n",
		"media-type image/png\n",
		"download-path /wiki/download/attachments/123/diagram%20one.png?api=v2\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("pointer missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "modificationDate") {
		t.Fatalf("pointer retained volatile download query:\n%s", text)
	}
}

func TestSafeAttachmentName(t *testing.T) {
	tests := []struct {
		title string
		id    string
		want  string
	}{
		{title: "diagram.png", id: "10", want: "diagram.png"},
		{title: "../../secret", id: "11", want: ".._.._secret"},
		{title: "a\\b\n.txt", id: "12", want: "a_b_.txt"},
		{title: "..", id: "13", want: "attachment-13"},
		{title: "", id: "14", want: "attachment-14"},
	}

	for _, test := range tests {
		if got := safeAttachmentName(test.title, test.id); got != test.want {
			t.Errorf("safeAttachmentName(%q, %q) = %q, want %q", test.title, test.id, got, test.want)
		}
	}
}

func TestAttachmentsUseConfiguredAPIPathButDownloadDoesNot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/custom/api/2.0/content/1/child/attachment":
			writeJSON(t, w, map[string]any{
				"results": []any{map[string]any{
					"id": "10", "title": "diagram.png",
					"_links": map[string]any{"download": "/download/attachments/1/diagram.png"},
				}},
			})
		case "/download/attachments/1/diagram.png":
			_, _ = w.Write([]byte("image data"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret-token")
	client.SetAPIPath("custom/api", "2.0")
	attachments, err := client.FetchAttachments("1")
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d", len(attachments))
	}
	data, err := client.DownloadAttachment(attachments[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "image data" {
		t.Fatalf("attachment data = %q", data)
	}
}
