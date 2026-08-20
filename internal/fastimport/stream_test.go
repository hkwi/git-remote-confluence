package fastimport

import (
	"bytes"
	"testing"

	"github.com/hkwi/git-remote-confluence/internal/confluencetypes"
)

func TestBuildStreamContainsPageFiles(t *testing.T) {
	page := PageRecord{
		PageID:     "1",
		Title:      "吾輩は猫である",
		Status:     "current",
		SpaceKey:   "ABC",
		Version:    confluencetypes.Version{Number: 3, When: "2025-01-02T03:04:05.000Z"},
		Links:      map[string]string{"webui": "https://cf.example.test/pages/viewpage.action?pageId=1"},
		StorageXML: "<p>吾輩は猫である。名前はまだ無い。</p>",
		Attachments: []AttachmentRecord{{
			ID:      "10",
			Title:   "挿絵.png",
			Path:    "1/attachments/挿絵.png",
			Pointer: []byte("version pointer/v1\nattachment-id 10\nattachment-version 2\n"),
		}},
	}

	stream := BuildStream(DefaultBranch, Location{RootType: "page", RootValue: "1"}, []PageRecord{page})
	for _, expected := range [][]byte{
		[]byte("M 100644 inline .gitattributes\n"),
		[]byte(AttributesContent),
		[]byte("M 100644 inline 1.md\n"),
		[]byte("M 100644 inline 1.yml\n"),
		[]byte("M 100644 inline 1/attachments/挿絵.png\n"),
		[]byte("attachment-id 10\n"),
		[]byte("attachment-version 2\n"),
		[]byte(`storage_xml: "1.md"`),
		[]byte("number: 3\n"),
	} {
		if !bytes.Contains(stream, expected) {
			t.Fatalf("stream did not contain %q\n%s", expected, stream)
		}
	}
}

func TestBuildAttachmentStreamContainsVersionHistory(t *testing.T) {
	attachment := AttachmentRecord{
		ID: "10", Title: "diagram.png",
		Versions: []AttachmentVersionRecord{
			{Version: confluencetypes.Version{Number: 1, When: "2025-01-01T00:00:00Z"}, Data: []byte("one")},
			{Version: confluencetypes.Version{Number: 2, When: "2025-01-02T00:00:00Z"}, Data: []byte("two")},
		},
	}
	stream := BuildAttachmentStream(DefaultBranch, attachment)
	for _, expected := range [][]byte{
		[]byte("Import Confluence attachment 10 version 1\n"),
		[]byte("Import Confluence attachment 10 version 2\n"),
		[]byte("from :1\n"),
		[]byte("M 100644 inline diagram.png\n"),
	} {
		if !bytes.Contains(stream, expected) {
			t.Fatalf("stream did not contain %q\n%s", expected, stream)
		}
	}
}
