package pagefiles

import "testing"

func TestParseMetadataYAML(t *testing.T) {
	metadata, err := ParseMetadataYAML([]byte(`page_id: "1"
title: "吾輩は猫である"
space_key: "ABC"
version:
  number: 7
files:
  storage_xml: "1.md"
  metadata: "1.yml"
sync:
  storage_sha256: "abc123"
`))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.PageID != "1" {
		t.Fatalf("PageID = %q", metadata.PageID)
	}
	if metadata.Title != "吾輩は猫である" {
		t.Fatalf("Title = %q", metadata.Title)
	}
	if metadata.SpaceKey != "ABC" {
		t.Fatalf("SpaceKey = %q", metadata.SpaceKey)
	}
	if metadata.VersionNumber != 7 {
		t.Fatalf("VersionNumber = %d", metadata.VersionNumber)
	}
	if metadata.StoragePath != "1.md" {
		t.Fatalf("StoragePath = %q", metadata.StoragePath)
	}
	if metadata.StorageSHA256 != "abc123" {
		t.Fatalf("StorageSHA256 = %q", metadata.StorageSHA256)
	}
}

func TestParseMetadataYAMLRequiresSyncFields(t *testing.T) {
	_, err := ParseMetadataYAML([]byte(`page_id: "1"
version:
  number: 7
files:
  storage_xml: "1.md"
`))
	if err == nil {
		t.Fatal("expected error")
	}
}
