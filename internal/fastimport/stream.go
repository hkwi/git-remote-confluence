package fastimport

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"

	"git-remote-confluence/internal/confluencetypes"
)

const DefaultBranch = "refs/heads/main"
const AttributesPath = ".gitattributes"
const AttributesContent = "*.md filter=confluence-storage diff=markdown\n"

type Location struct {
	RootType  string
	RootValue string
}

type PageRecord struct {
	PageID     string
	Title      string
	Status     string
	SpaceKey   string
	ParentID   string
	ChildIDs   []string
	Version    confluencetypes.Version
	Links      map[string]string
	StorageXML string
	PathDir    string
}

func (p PageRecord) ContentPath() string {
	return joinPath(p.PathDir, p.PageID+".md")
}

func (p PageRecord) MetadataPath() string {
	return joinPath(p.PathDir, p.PageID+".yml")
}

func BuildStream(branch string, location Location, pages []PageRecord) []byte {
	return BuildStreamWithProgress(branch, location, pages, false)
}

func BuildStreamWithProgress(branch string, location Location, pages []PageRecord, progress bool) []byte {
	var out bytes.Buffer
	if progress {
		appendProgress(&out, "confluence: importing %d pages", len(pages))
		for _, page := range pages {
			appendProgress(&out, "confluence: importing page %s %s", page.PageID, page.Title)
		}
	}
	fmt.Fprintf(&out, "commit %s\n", branch)
	fmt.Fprintf(&out, "committer Confluence <confluence@example.invalid> %d +0000\n", commitTimestamp(pages))
	appendData(&out, []byte(commitMessage(location)))
	out.WriteString("deleteall\n")
	appendFile(&out, AttributesPath, []byte(AttributesContent))

	for _, page := range pages {
		appendFile(&out, page.ContentPath(), []byte(page.StorageXML))
		appendFile(&out, page.MetadataPath(), []byte(PageMetadataYAML(location, page)))
	}

	out.WriteByte('\n')
	if progress {
		appendProgress(&out, "confluence: done")
	}
	out.WriteString("done\n")
	return out.Bytes()
}

func SelectBranch(refs []string) string {
	for _, ref := range refs {
		if ref != "HEAD" && strings.HasPrefix(ref, "refs/heads/") {
			return ref
		}
	}
	return DefaultBranch
}

func PageMetadataYAML(location Location, page PageRecord) string {
	hash := sha256.Sum256([]byte(page.StorageXML))
	root := yamlMap{
		{"page_id", page.PageID},
		{"title", page.Title},
		{"status", page.Status},
		{"space_key", page.SpaceKey},
		{"parent_id", emptyAsNil(page.ParentID)},
		{"children", page.ChildIDs},
		{"version", yamlMap{
			{"number", page.Version.Number},
			{"when", page.Version.When},
			{"minor_edit", page.Version.MinorEdit},
			{"message", emptyAsNil(page.Version.Message)},
			{"by", userMap(page.Version.By)},
		}},
		{"links", stringMap(page.Links)},
		{"files", yamlMap{
			{"storage_xml", page.ContentPath()},
			{"metadata", page.MetadataPath()},
		}},
		{"sync", yamlMap{
			{"source", "confluence"},
			{"root_type", location.RootType},
			{"root", location.RootValue},
			{"storage_sha256", fmt.Sprintf("%x", hash[:])},
		}},
	}
	return dumpYAML(compactMap(root))
}

func appendFile(out *bytes.Buffer, path string, data []byte) {
	fmt.Fprintf(out, "M 100644 inline %s\n", quotePath(path))
	appendData(out, data)
}

func appendData(out *bytes.Buffer, data []byte) {
	fmt.Fprintf(out, "data %d\n", len(data))
	out.Write(data)
	out.WriteByte('\n')
}

func appendProgress(out *bytes.Buffer, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	message = strings.NewReplacer("\n", " ", "\r", " ").Replace(message)
	out.WriteString("progress ")
	out.WriteString(message)
	out.WriteByte('\n')
}

func quotePath(path string) string {
	if path != "" && !strings.HasPrefix(path, "\"") && !strings.ContainsAny(path, " \n\\") {
		return path
	}
	escaped := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n").Replace(path)
	return `"` + escaped + `"`
}

func commitMessage(location Location) string {
	return "Import Confluence " + location.RootType + " " + location.RootValue + "\n"
}

func commitTimestamp(pages []PageRecord) int64 {
	var max int64
	for _, page := range pages {
		when := page.Version.When
		if when == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, when)
		if err != nil {
			continue
		}
		if unix := parsed.Unix(); unix > max {
			max = unix
		}
	}
	return max
}

func joinPath(parts ...string) string {
	var kept []string
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "/")
}

type yamlPair struct {
	key   string
	value any
}

type yamlMap []yamlPair

func dumpYAML(value any) string {
	var lines []string
	emitYAML(value, &lines, 0)
	return strings.Join(lines, "\n") + "\n"
}

func emitYAML(value any, lines *[]string, indent int) {
	prefix := strings.Repeat(" ", indent)
	switch typed := value.(type) {
	case yamlMap:
		for _, pair := range typed {
			switch child := pair.value.(type) {
			case yamlMap:
				*lines = append(*lines, prefix+pair.key+":")
				emitYAML(child, lines, indent+2)
			case []string:
				*lines = append(*lines, prefix+pair.key+":")
				emitYAML(child, lines, indent+2)
			default:
				*lines = append(*lines, prefix+pair.key+": "+yamlScalar(child))
			}
		}
	case []string:
		if len(typed) == 0 {
			*lines = append(*lines, prefix+"[]")
			return
		}
		for _, item := range typed {
			*lines = append(*lines, prefix+"- "+yamlScalar(item))
		}
	default:
		*lines = append(*lines, prefix+yamlScalar(typed))
	}
}

func yamlScalar(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		text := fmt.Sprint(typed)
		if text == "" {
			return `""`
		}
		escaped := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n").Replace(text)
		return `"` + escaped + `"`
	}
}

func compactMap(value yamlMap) yamlMap {
	var compacted yamlMap
	for _, pair := range value {
		if isEmpty(pair.value) {
			continue
		}
		if nested, ok := pair.value.(yamlMap); ok {
			pair.value = compactMap(nested)
			if isEmpty(pair.value) {
				continue
			}
		}
		compacted = append(compacted, pair)
	}
	return compacted
}

func isEmpty(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case yamlMap:
		return len(typed) == 0
	case []string:
		return false
	default:
		return false
	}
}

func emptyAsNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func userMap(user confluencetypes.User) any {
	value := compactMap(yamlMap{
		{"type", user.Type},
		{"username", user.Username},
		{"user_key", user.UserKey},
		{"account_id", user.AccountID},
		{"display_name", user.DisplayName},
	})
	if len(value) == 0 {
		return nil
	}
	return value
}

func stringMap(values map[string]string) yamlMap {
	var result yamlMap
	for _, key := range []string{"webui", "tinyui", "self"} {
		if value := values[key]; value != "" {
			result = append(result, yamlPair{key, value})
		}
	}
	return result
}
