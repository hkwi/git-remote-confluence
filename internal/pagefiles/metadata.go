package pagefiles

import (
	"fmt"
	"strconv"
	"strings"
)

type Metadata struct {
	PageID        string
	Title         string
	SpaceKey      string
	VersionNumber int
	StoragePath   string
	StorageSHA256 string
}

func ParseMetadataYAML(data []byte) (Metadata, error) {
	values := map[string]string{}
	var stack []string

	lines := strings.Split(string(data), "\n")
	for lineNumber, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		trimmedLeft := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmedLeft, "#") || strings.HasPrefix(trimmedLeft, "- ") || trimmedLeft == "[]" {
			continue
		}
		if len(line)-len(trimmedLeft) != countLeadingSpaces(line) {
			return Metadata{}, fmt.Errorf("metadata line %d uses unsupported indentation", lineNumber+1)
		}

		indent := countLeadingSpaces(line)
		if indent%2 != 0 {
			return Metadata{}, fmt.Errorf("metadata line %d uses unsupported indentation", lineNumber+1)
		}
		level := indent / 2
		if level > len(stack) {
			return Metadata{}, fmt.Errorf("metadata line %d skips an indentation level", lineNumber+1)
		}
		stack = stack[:level]

		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) != 2 {
			return Metadata{}, fmt.Errorf("metadata line %d is not a key/value pair", lineNumber+1)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return Metadata{}, fmt.Errorf("metadata line %d has an empty key", lineNumber+1)
		}
		if value == "" {
			stack = append(stack, key)
			continue
		}

		path := append(append([]string(nil), stack...), key)
		values[strings.Join(path, ".")] = unquoteScalar(value)
	}

	versionNumber, err := strconv.Atoi(values["version.number"])
	if err != nil || versionNumber <= 0 {
		return Metadata{}, fmt.Errorf("metadata version.number is required")
	}

	metadata := Metadata{
		PageID:        values["page_id"],
		Title:         values["title"],
		SpaceKey:      values["space_key"],
		VersionNumber: versionNumber,
		StoragePath:   values["files.storage_xml"],
		StorageSHA256: values["sync.storage_sha256"],
	}
	if metadata.PageID == "" {
		return Metadata{}, fmt.Errorf("metadata page_id is required")
	}
	if metadata.StoragePath == "" {
		return Metadata{}, fmt.Errorf("metadata files.storage_xml is required")
	}
	if metadata.StorageSHA256 == "" {
		return Metadata{}, fmt.Errorf("metadata sync.storage_sha256 is required")
	}
	return metadata, nil
}

func countLeadingSpaces(value string) int {
	return len(value) - len(strings.TrimLeft(value, " "))
}

func unquoteScalar(value string) string {
	if value == "null" {
		return ""
	}
	unquoted, err := strconv.Unquote(value)
	if err == nil {
		return unquoted
	}
	return value
}
