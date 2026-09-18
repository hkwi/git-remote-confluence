package confluence

import "github.com/hkwi/git-remote-confluence/internal/fastimport"

type FetchOptions struct {
	// SkipUnavailableChildren permits an incomplete page tree. Callers updating
	// an existing snapshot must leave this false to avoid interpreting missing
	// content as a deletion.
	SkipUnavailableChildren bool
	Progress                ProgressFunc
	Warning                 ProgressFunc
}

type SkippedPage struct {
	PageID     string
	ParentID   string
	StatusCode int
}

type ChildListError struct {
	PageID     string
	ParentID   string
	StatusCode int
}

type FetchResult struct {
	Pages                 []fastimport.PageRecord
	SkippedPages          []SkippedPage
	UnavailableChildLists []ChildListError
}

func FetchPagesWithOptions(client *Client, location Location, options FetchOptions) (FetchResult, error) {
	location, err := ResolveLocation(client, location, options.Progress)
	if err != nil {
		return FetchResult{}, err
	}

	switch location.RootType {
	case "page":
		return fetchPageTree(client, location.RootValue, options)
	case "space":
		pages, err := fetchSpaceTree(client, location.RootValue, options.Progress)
		return FetchResult{Pages: pages}, err
	default:
		return FetchResult{}, ErrUnsupportedRoot(location.RootType)
	}
}
