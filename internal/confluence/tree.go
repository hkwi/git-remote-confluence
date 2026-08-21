package confluence

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"

	"github.com/hkwi/git-remote-confluence/internal/fastimport"
)

type ProgressFunc func(format string, args ...any)

func FetchPages(client *Client, location Location) ([]fastimport.PageRecord, error) {
	return FetchPagesWithProgress(client, location, nil)
}

func FetchPagesWithProgress(client *Client, location Location, progress ProgressFunc) ([]fastimport.PageRecord, error) {
	location, err := ResolveLocation(client, location, progress)
	if err != nil {
		return nil, err
	}

	switch location.RootType {
	case "page":
		return fetchPageTree(client, location.RootValue, progress)
	case "space":
		return fetchSpaceTree(client, location.RootValue, progress)
	default:
		return nil, ErrUnsupportedRoot(location.RootType)
	}
}

func ResolveLocation(client *Client, location Location, progress ProgressFunc) (Location, error) {
	if location.RootType != "page" || location.RootValue != "" {
		return location, nil
	}
	if location.SpaceKey == "" || location.PageTitle == "" {
		return Location{}, ErrUnresolvedPageLocation{}
	}

	report(progress, "resolving page %q in space %s", location.PageTitle, location.SpaceKey)
	page, err := client.FetchPageByTitle(location.SpaceKey, location.PageTitle)
	if err != nil {
		return Location{}, err
	}
	location.RootValue = page.ID
	report(progress, "resolved page %q in space %s to page %s", location.PageTitle, location.SpaceKey, location.RootValue)
	return location, nil
}

type ErrUnsupportedRoot string

func (e ErrUnsupportedRoot) Error() string {
	return "unsupported root type: " + string(e)
}

type ErrUnresolvedPageLocation struct{}

func (e ErrUnresolvedPageLocation) Error() string {
	return "page root must identify a pageId or a display page title"
}

func fetchPageTree(client *Client, rootID string, progress ProgressFunc) ([]fastimport.PageRecord, error) {
	var records []fastimport.PageRecord
	seen := map[string]bool{}

	var visit func(pageID, parentID, pathDir string) error
	visit = func(pageID, parentID, pathDir string) error {
		if seen[pageID] {
			return nil
		}
		seen[pageID] = true

		report(progress, "fetching page %s", pageID)
		page, err := client.FetchPage(pageID)
		if err != nil {
			return err
		}
		children, err := client.FetchChildren(pageID)
		if err != nil {
			return err
		}

		childIDs := make([]string, 0, len(children))
		for _, child := range children {
			if child.ID != "" {
				childIDs = append(childIDs, child.ID)
			}
		}
		report(progress, "page %s has %d child pages", pageID, len(childIDs))

		record := pageRecord(page, parentID, childIDs, pathDir, client.BaseURL)
		attachments, err := fetchAttachments(client, record, progress)
		if err != nil {
			return err
		}
		record.Attachments = attachments
		records = append(records, record)

		childPathDir := joinPath(pathDir, record.PageID)
		for _, childID := range childIDs {
			if err := visit(childID, record.PageID, childPathDir); err != nil {
				return err
			}
		}
		return nil
	}

	if err := visit(rootID, "", ""); err != nil {
		return nil, err
	}
	return records, nil
}

func fetchSpaceTree(client *Client, spaceKey string, progress ProgressFunc) ([]fastimport.PageRecord, error) {
	report(progress, "fetching space %s", spaceKey)
	pages, err := client.FetchSpacePages(spaceKey)
	if err != nil {
		return nil, err
	}
	report(progress, "space %s returned %d pages", spaceKey, len(pages))

	byID := map[string]Page{}
	for _, page := range pages {
		if page.ID != "" {
			byID[page.ID] = page
		}
	}

	children := map[string][]string{"": {}}
	parentByID := map[string]string{}
	for pageID, page := range byID {
		parentID := directParentID(page, byID)
		parentByID[pageID] = parentID
		children[parentID] = append(children[parentID], pageID)
		if _, ok := children[pageID]; !ok {
			children[pageID] = nil
		}
	}

	for _, childIDs := range children {
		sort.Slice(childIDs, func(i, j int) bool {
			left := byID[childIDs[i]]
			right := byID[childIDs[j]]
			if left.Title == right.Title {
				return left.ID < right.ID
			}
			return left.Title < right.Title
		})
	}

	var records []fastimport.PageRecord
	seen := map[string]bool{}
	var visit func(pageID, pathDir string) error
	visit = func(pageID, pathDir string) error {
		if seen[pageID] {
			return nil
		}
		seen[pageID] = true

		page := byID[pageID]
		record := pageRecord(page, parentByID[pageID], children[pageID], pathDir, client.BaseURL)
		attachments, err := fetchAttachments(client, record, progress)
		if err != nil {
			return err
		}
		record.Attachments = attachments
		records = append(records, record)

		childPathDir := joinPath(pathDir, record.PageID)
		for _, childID := range children[pageID] {
			if err := visit(childID, childPathDir); err != nil {
				return err
			}
		}
		return nil
	}

	for _, rootID := range children[""] {
		if err := visit(rootID, ""); err != nil {
			return nil, err
		}
	}

	var ids []string
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id, ""); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func fetchAttachments(client *Client, page fastimport.PageRecord, progress ProgressFunc) ([]fastimport.AttachmentRecord, error) {
	attachments, err := client.FetchAttachments(page.PageID)
	if err != nil {
		return nil, fmt.Errorf("fetch attachments for page %s: %w", page.PageID, err)
	}
	report(progress, "page %s has %d attachments", page.PageID, len(attachments))

	result := make([]fastimport.AttachmentRecord, 0, len(attachments))
	usedPaths := map[string]bool{}
	for _, attachment := range attachments {
		name := safeAttachmentName(attachment.Title, attachment.ID)
		path := joinPath(page.PathDir, page.PageID, "attachments", name)
		if usedPaths[path] {
			name = safeAttachmentName(attachment.ID+"-"+attachment.Title, attachment.ID)
			path = joinPath(page.PathDir, page.PageID, "attachments", name)
		}
		usedPaths[path] = true
		pointer, err := attachmentPointer(client.BaseURL, page.PageID, attachment)
		if err != nil {
			return nil, fmt.Errorf("build pointer for attachment %s (%q): %w", attachment.ID, attachment.Title, err)
		}
		result = append(result, fastimport.AttachmentRecord{
			ID:      attachment.ID,
			Title:   name,
			Path:    path,
			Pointer: pointer,
		})
		report(progress, "recorded attachment pointer %s %s version %d", attachment.ID, attachment.Title, attachment.Version.Number)
	}
	return result, nil
}

const attachmentPointerVersion = "https://github.com/hkwi/git-remote-confluence/spec/attachment/v1"

type attachmentPointerYAML struct {
	Version           string `yaml:"version"`
	SourceURL         string `yaml:"source"`
	PageID            string `yaml:"page_id"`
	AttachmentID      string `yaml:"attachment_id"`
	AttachmentVersion int    `yaml:"attachment_version"`
	Filename          string `yaml:"filename"`
	Size              int64  `yaml:"size"`
	MediaType         string `yaml:"media_type,omitempty"`
	DownloadPath      string `yaml:"download_path"`
}

func attachmentPointer(baseURL, pageID string, attachment Attachment) ([]byte, error) {
	if attachment.ID == "" {
		return nil, fmt.Errorf("attachment id is required")
	}
	if attachment.Version.Number <= 0 {
		return nil, fmt.Errorf("attachment version is required")
	}
	downloadPath, err := stableDownloadPath(baseURL, attachment.Links["download"])
	if err != nil {
		return nil, err
	}
	filename := safeAttachmentName(attachment.Title, attachment.ID)
	pointer, err := yaml.Marshal(attachmentPointerYAML{
		Version:           attachmentPointerVersion,
		SourceURL:         strings.TrimRight(baseURL, "/"),
		PageID:            pageID,
		AttachmentID:      attachment.ID,
		AttachmentVersion: attachment.Version.Number,
		Filename:          filename,
		Size:              attachment.Extensions.FileSize,
		MediaType:         attachment.Extensions.MediaType,
		DownloadPath:      downloadPath,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal attachment pointer YAML: %w", err)
	}
	return pointer, nil
}

func stableDownloadPath(baseURL, download string) (string, error) {
	if download == "" {
		return "", fmt.Errorf("download link is required")
	}
	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return "", fmt.Errorf("invalid Confluence base URL: %w", err)
	}
	link, err := url.Parse(download)
	if err != nil {
		return "", fmt.Errorf("invalid download link: %w", err)
	}
	resolved := base.ResolveReference(link)
	if resolved.Scheme != base.Scheme || resolved.Host != base.Host {
		return "", fmt.Errorf("download link points outside Confluence origin")
	}
	query := resolved.Query()
	stableQuery := url.Values{}
	if api := query.Get("api"); api != "" {
		stableQuery.Set("api", api)
	}
	resolved.RawQuery = stableQuery.Encode()
	resolved.Fragment = ""
	path := resolved.EscapedPath()
	if path == "" || !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("download link has no absolute path")
	}
	if resolved.RawQuery != "" {
		path += "?" + resolved.RawQuery
	}
	return path, nil
}

func FetchAttachmentRepository(client *Client, attachmentID string, progress ProgressFunc) (fastimport.AttachmentRecord, error) {
	report(progress, "fetching attachment %s", attachmentID)
	attachment, err := client.FetchAttachment(attachmentID)
	if err != nil {
		return fastimport.AttachmentRecord{}, fmt.Errorf("fetch attachment %s: %w", attachmentID, err)
	}
	return fetchAttachmentRecord(client, attachment, "", progress)
}

func fetchAttachmentRecord(client *Client, attachment Attachment, path string, progress ProgressFunc) (fastimport.AttachmentRecord, error) {
	versions, err := client.FetchAttachmentVersions(attachment)
	if err != nil {
		return fastimport.AttachmentRecord{}, fmt.Errorf("fetch versions for attachment %s (%q): %w", attachment.ID, attachment.Title, err)
	}
	if len(versions) == 0 {
		return fastimport.AttachmentRecord{}, fmt.Errorf("Confluence attachment %s (%q) has no versions", attachment.ID, attachment.Title)
	}

	record := fastimport.AttachmentRecord{
		ID:    attachment.ID,
		Title: safeAttachmentName(attachment.Title, attachment.ID),
		Path:  path,
	}
	for _, version := range versions {
		data, err := client.DownloadAttachmentVersion(attachment, version.Number)
		if err != nil {
			return fastimport.AttachmentRecord{}, fmt.Errorf("download attachment %s (%q) version %d: %w", attachment.ID, attachment.Title, version.Number, err)
		}
		record.Versions = append(record.Versions, fastimport.AttachmentVersionRecord{Version: version, Data: data})
		report(progress, "downloaded attachment %s %s version %d (%d bytes)", attachment.ID, attachment.Title, version.Number, len(data))
	}
	return record, nil
}

func safeAttachmentName(title, id string) string {
	name := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return '_'
		}
		return r
	}, title)
	if name == "" || name == "." || name == ".." {
		name = "attachment-" + id
	}
	return name
}

func report(progress ProgressFunc, format string, args ...any) {
	if progress != nil {
		progress(format, args...)
	}
}

func pageRecord(page Page, parentID string, childIDs []string, pathDir, baseURL string) fastimport.PageRecord {
	return fastimport.PageRecord{
		PageID:     page.ID,
		Title:      page.Title,
		Status:     page.Status,
		SpaceKey:   page.Space.Key,
		ParentID:   parentID,
		ChildIDs:   append([]string(nil), childIDs...),
		Version:    page.Version,
		Links:      pageLinks(page, baseURL),
		StorageXML: page.Body.Storage.Value,
		PathDir:    pathDir,
	}
}

func pageLinks(page Page, baseURL string) map[string]string {
	result := map[string]string{}
	for _, key := range []string{"webui", "tinyui", "self"} {
		value := page.Links[key]
		if value == "" {
			continue
		}
		result[key] = resolveLink(baseURL, value)
	}
	return result
}

func resolveLink(baseURL, value string) string {
	if stringsHasHTTPPrefix(value) {
		return value
	}
	if value == "" || value[0] != '/' {
		value = "/" + value
	}
	return baseURL + value
}

func directParentID(page Page, knownPages map[string]Page) string {
	for index := len(page.Ancestors) - 1; index >= 0; index-- {
		id := page.Ancestors[index].ID
		if _, ok := knownPages[id]; ok {
			return id
		}
	}
	return ""
}

func joinPath(parts ...string) string {
	var joined string
	for _, part := range parts {
		if part == "" {
			continue
		}
		if joined != "" {
			joined += "/"
		}
		joined += part
	}
	return joined
}

func stringsHasHTTPPrefix(value string) bool {
	return len(value) >= 7 && (value[:7] == "http://" || len(value) >= 8 && value[:8] == "https://")
}
