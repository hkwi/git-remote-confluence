package fastimport

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hkwi/git-remote-confluence/internal/confluencetypes"
)

const DefaultBranch = "refs/heads/main"
const AttributesPath = ".gitattributes"
const AttributesContent = "*.md filter=confluence-storage diff=markdown\n"

type Location struct {
	RootType  string
	RootValue string
}

type PageRecord struct {
	PageID              string
	Title               string
	Status              string
	SpaceKey            string
	ParentID            string
	ChildIDs            []string
	SkippedChildIDs     []string
	ChildrenErrorStatus int
	// AttachmentsErrorStatus records that Confluence refused part or all of the
	// attachment list, so the imported tree is knowingly incomplete.
	AttachmentsErrorStatus int
	// CommentsErrorStatus records that Confluence refused part or all of the
	// comment list, so the imported comment thread is knowingly incomplete.
	CommentsErrorStatus int
	Version             confluencetypes.Version
	Links               map[string]string
	StorageXML          string
	PathDir             string
	Attachments         []AttachmentRecord
	// Comments is the page's comment thread as a single YAML file.
	Comments []CommentRecord
}

type AttachmentRecord struct {
	Path string
	Data []byte
}

type CommentRecord struct {
	ID      string
	Status  string
	Version confluencetypes.Version
	// Ancestors lists the ancestor comment IDs that precede this comment in
	// the thread, excluding the owning page, so nested replies are preserved.
	Ancestors []string
	Body      string // storage-format XML
}

func (p PageRecord) ContentPath() string {
	return joinPath(p.PathDir, p.PageID+".md")
}

func (p PageRecord) MetadataPath() string {
	return joinPath(p.PathDir, p.PageID+".yml")
}

func (p PageRecord) AttachmentsDir() string {
	return joinPath(p.PathDir, p.PageID, "attachments")
}

// CommentsPath returns the single file that holds the whole comment thread for
// the page.
func (p PageRecord) CommentsPath() string {
	return joinPath(p.PathDir, p.PageID, "comments.yml")
}

// BuildStream is a convenience wrapper that produces a root import commit
// (no parent). Use BuildStreamWithParent to chain repeated imports onto the
// previous local tip.
func BuildStream(branch string, location Location, pages []PageRecord) []byte {
	return BuildStreamWithParent(branch, location, pages, "", false)
}

// BuildStreamWithProgress is a convenience wrapper that produces a root import
// commit (no parent) while optionally emitting progress lines.
func BuildStreamWithProgress(branch string, location Location, pages []PageRecord, progress bool) []byte {
	return BuildStreamWithParent(branch, location, pages, "", progress)
}

// BuildStreamWithParent builds a fast-import stream for a fresh import of
// pages onto branch. When parent is non-empty it is emitted as a "from" line,
// chaining the new commit onto the existing local tip so that repeated fetches
// remain fast-forward instead of being rejected as a rewrite of history.
func BuildStreamWithParent(branch string, location Location, pages []PageRecord, parent string, progress bool) []byte {
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
	if parent != "" {
		fmt.Fprintf(&out, "from %s\n", parent)
	}
	out.WriteString("deleteall\n")
	appendFile(&out, AttributesPath, []byte(AttributesContent))

	for _, page := range pages {
		appendFile(&out, page.ContentPath(), []byte(page.StorageXML))
		appendFile(&out, page.MetadataPath(), []byte(PageMetadataYAML(location, page)))
		for _, attachment := range page.Attachments {
			appendFile(&out, attachment.Path, attachment.Data)
		}
		if len(page.Comments) > 0 {
			appendFile(&out, page.CommentsPath(), []byte(CommentsYAML(page.Comments)))
		}
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
	files := yamlMap{
		{"storage_xml", page.ContentPath()},
		{"metadata", page.MetadataPath()},
	}
	if len(page.Comments) > 0 {
		files = append(files, yamlPair{"comments", page.CommentsPath()})
	}
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
		{"files", files},
		{"sync", yamlMap{
			{"source", "confluence"},
			{"root_type", location.RootType},
			{"root", location.RootValue},
			{"storage_sha256", fmt.Sprintf("%x", hash[:])},
		}},
	}
	if len(page.SkippedChildIDs) > 0 {
		root = append(root, yamlPair{"skipped_children", page.SkippedChildIDs})
	}
	if page.ChildrenErrorStatus != 0 {
		root = append(root, yamlPair{"children_error", yamlMap{{"http_status", page.ChildrenErrorStatus}}})
	}
	if page.AttachmentsErrorStatus != 0 {
		root = append(root, yamlPair{"attachments_error", yamlMap{{"http_status", page.AttachmentsErrorStatus}}})
	}
	if page.CommentsErrorStatus != 0 {
		root = append(root, yamlPair{"comments_error", yamlMap{{"http_status", page.CommentsErrorStatus}}})
	}
	return dumpYAML(compactMap(root))
}

// CommentsYAML renders the page's whole comment thread as one YAML document:
// a list of comments, each preserving its Confluence id, status, version
// (including author), ancestor comment ids, and storage-format body.
func CommentsYAML(comments []CommentRecord) string {
	list := make([]yamlMap, 0, len(comments))
	for _, comment := range comments {
		list = append(list, yamlMap{
			{"id", comment.ID},
			{"status", emptyAsNil(comment.Status)},
			{"version", yamlMap{
				{"number", comment.Version.Number},
				{"when", emptyAsNil(comment.Version.When)},
				{"minor_edit", comment.Version.MinorEdit},
				{"message", emptyAsNil(comment.Version.Message)},
				{"by", userMap(comment.Version.By)},
			}},
			{"ancestors", comment.Ancestors},
			{"body", comment.Body},
		})
	}
	return dumpYAML(yamlMap{{"comments", list}})
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
			case []yamlMap:
				*lines = append(*lines, prefix+pair.key+":")
				emitYAMLList(child, lines, indent)
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

func emitYAMLList(list []yamlMap, lines *[]string, indent int) {
	if len(list) == 0 {
		*lines = append(*lines, strings.Repeat(" ", indent+2)+"[]")
		return
	}
	for _, item := range list {
		emitYAMLListItem(item, lines, indent+2)
	}
}

func emitYAMLListItem(item yamlMap, lines *[]string, indent int) {
	prefix := strings.Repeat(" ", indent)
	for index, pair := range item {
		keyPrefix := strings.Repeat(" ", indent+2)
		if index == 0 {
			keyPrefix = prefix + "- "
		}
		switch child := pair.value.(type) {
		case yamlMap:
			*lines = append(*lines, keyPrefix+pair.key+":")
			emitYAML(child, lines, indent+4)
		case []string:
			*lines = append(*lines, keyPrefix+pair.key+":")
			emitYAML(child, lines, indent+4)
		case []yamlMap:
			*lines = append(*lines, keyPrefix+pair.key+":")
			emitYAMLList(child, lines, indent+2)
		default:
			*lines = append(*lines, keyPrefix+pair.key+": "+yamlScalar(child))
		}
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
		if nested, ok := pair.value.([]yamlMap); ok {
			for i := range nested {
				nested[i] = compactMap(nested[i])
			}
			if len(nested) == 0 {
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
	case []yamlMap:
		return len(typed) == 0
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
