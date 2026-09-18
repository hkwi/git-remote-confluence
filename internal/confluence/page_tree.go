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
			var apiErr *APIError
			if options.SkipUnavailableChildren && parentID != "" && errors.As(err, &apiErr) &&
				(apiErr.StatusCode == http.StatusForbidden || apiErr.StatusCode == http.StatusNotFound) {
				skipped[pageID] = true
				result.SkippedPages = append(result.SkippedPages, SkippedPage{
					PageID: pageID, ParentID: parentID, StatusCode: apiErr.StatusCode,
				})
				report(options.Warning, "skipping page %s under parent %s and its subtree: HTTP %d (missing or not permitted)", pageID, parentID, apiErr.StatusCode)
				return nil
			}
			return fmt.Errorf("fetch page %s: %w", pageID, err)
		}
		children, err := client.FetchChildren(pageID)
		if err != nil {
			return fmt.Errorf("fetch children of page %s: %w", pageID, err)
		}

		var childIDs []string
		for _, child := range children {
			if child.ID != "" {
				childIDs = append(childIDs, child.ID)
			}
		}
		report(options.Progress, "page %s has %d child pages", pageID, len(childIDs))

		record := pageRecord(page, parentID, childIDs, pathDir, client.BaseURL)
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
