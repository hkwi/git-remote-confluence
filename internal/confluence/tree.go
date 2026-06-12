package confluence

import (
	"sort"

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
	var visit func(pageID, pathDir string)
	visit = func(pageID, pathDir string) {
		if seen[pageID] {
			return
		}
		seen[pageID] = true

		page := byID[pageID]
		record := pageRecord(page, parentByID[pageID], children[pageID], pathDir, client.BaseURL)
		records = append(records, record)

		childPathDir := joinPath(pathDir, record.PageID)
		for _, childID := range children[pageID] {
			visit(childID, childPathDir)
		}
	}

	for _, rootID := range children[""] {
		visit(rootID, "")
	}

	var ids []string
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		visit(id, "")
	}
	return records, nil
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
