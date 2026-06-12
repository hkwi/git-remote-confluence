package fastimport

import (
	"bytes"
	"testing"

	"git-remote-confluence/internal/confluencetypes"
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
	}

	stream := BuildStream(DefaultBranch, Location{RootType: "page", RootValue: "1"}, []PageRecord{page})
	for _, expected := range [][]byte{
		[]byte("M 100644 inline .gitattributes\n"),
		[]byte(AttributesContent),
		[]byte("M 100644 inline 1.md\n"),
		[]byte("M 100644 inline 1.yml\n"),
		[]byte(`storage_xml: "1.md"`),
		[]byte("number: 3\n"),
	} {
		if !bytes.Contains(stream, expected) {
			t.Fatalf("stream did not contain %q\n%s", expected, stream)
		}
	}
}
