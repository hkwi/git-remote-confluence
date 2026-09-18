package confluence

import (
	"errors"
	"fmt"
	"net/http"
)

func fetchPageTree(client *Client, rootID string, options FetchOptions) (FetchResult, error) {
	var result FetchResult
	seen := map[string]bool{}
	skipped := map[string]bool{}

	var visit func(pageID, parentID, pathDir string) error
	visit = func(pageID, parentID, pathDir string) error {
		if seen[pageID] {
			return nil
		}
		seen[pageID] = true

		report(options.Progress, "fetching page %s", pageID)
		page, err := client.FetchPage(pageID)
		if err != nil {
			if status := unavailableContentStatus(err); options.SkipUnavailableChildren && parentID != "" && status != 0 {
				skipped[pageID] = true
				result.SkippedPages = append(result.SkippedPages, SkippedPage{
					PageID: pageID, ParentID: parentID, StatusCode: status,
				})
				report(options.Warning, "skipping page %s under parent %s and its subtree: HTTP %d (missing or not permitted)", pageID, parentID, status)
				return nil
			}
			return fmt.Errorf("fetch page %s: %w", pageID, err)
		}
		children, err := client.FetchChildren(pageID)
		childrenErrorStatus := 0
		if err != nil {
			childrenErrorStatus = unavailableContentStatus(err)
			if !options.SkipUnavailableChildren || parentID == "" || childrenErrorStatus == 0 {
				return fmt.Errorf("fetch children of page %s: %w", pageID, err)
			}
			result.UnavailableChildLists = append(result.UnavailableChildLists, ChildListError{
				PageID: pageID, ParentID: parentID, StatusCode: childrenErrorStatus,
			})
			report(options.Warning, "incomplete child list for page %s under parent %s: HTTP %d; keeping page and any listed children, remaining descendants unknown", pageID, parentID, childrenErrorStatus)
		}

		var childIDs []string
		for _, child := range children {
			if child.ID != "" {
				childIDs = append(childIDs, child.ID)
			}
		}
		if childrenErrorStatus == 0 {
			report(options.Progress, "page %s has %d child pages", pageID, len(childIDs))
		}

		record := pageRecord(page, parentID, childIDs, pathDir, client.BaseURL)
		record.ChildrenErrorStatus = childrenErrorStatus
		attachments, err := fetchAttachments(client, record, options.Progress)
		if err != nil {
			return err
		}
		record.Attachments = attachments
		recordIndex := len(result.Pages)
		result.Pages = append(result.Pages, record)

		childPathDir := joinPath(pathDir, record.PageID)
		for _, childID := range childIDs {
			if err := visit(childID, record.PageID, childPathDir); err != nil {
				return err
			}
		}
		// Only imported children belong in children; preserve unavailable IDs
		// separately so the metadata does not imply that the tree is complete.
		result.Pages[recordIndex].ChildIDs = nil
		for _, childID := range childIDs {
			if skipped[childID] {
				result.Pages[recordIndex].SkippedChildIDs = append(result.Pages[recordIndex].SkippedChildIDs, childID)
			} else {
				result.Pages[recordIndex].ChildIDs = append(result.Pages[recordIndex].ChildIDs, childID)
			}
		}
		return nil
	}

	if err := visit(rootID, "", ""); err != nil {
		return FetchResult{}, err
	}
	return result, nil
}

func unavailableContentStatus(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusForbidden || apiErr.StatusCode == http.StatusNotFound) {
		return apiErr.StatusCode
	}
	return 0
}
