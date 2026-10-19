package confluence

import (
	"net/url"
	"sort"
	"strconv"

	"github.com/hkwi/git-remote-confluence/internal/confluencetypes"
)

// Comment is a Confluence comment on a page. The whole comment tree (top-level
// and replies) is fetched through the /descendant/comment endpoint, and each
// comment's storage-format body is preserved so the thread can be revisited or
// pushed later without lossy conversion.
type Comment struct {
	ID        string                  `json:"id"`
	Status    string                  `json:"status"`
	Version   confluencetypes.Version `json:"version"`
	Ancestors []Ancestor              `json:"ancestors"`
	Body      Body                    `json:"body"`
}

type commentListResponse struct {
	Results []Comment         `json:"results"`
	Links   map[string]string `json:"_links"`
}

// FetchComments returns every comment under pageID, including replies, ordered
// deterministically by comment id. Each comment's ancestors carry the full
// reply chain as parent comment ids (nearest parent first; the owning page is
// not included), which is enough to reconstruct the thread.
func (c *Client) FetchComments(pageID string) ([]Comment, error) {
	var comments []Comment
	start := 0
	const limit = 100

	for {
		values := url.Values{}
		values.Set("expand", "body.storage,version,version.by,ancestors")
		values.Set("start", strconv.Itoa(start))
		values.Set("limit", strconv.Itoa(limit))

		var response commentListResponse
		path := c.apiPath("content/" + url.PathEscape(pageID) + "/descendant/comment")
		if err := c.getJSON(path, values, &response); err != nil {
			return comments, err
		}
		comments = append(comments, response.Results...)
		if len(response.Results) == 0 || len(response.Results) < limit || response.Links["next"] == "" {
			break
		}
		start += len(response.Results)
	}

	sort.SliceStable(comments, func(i, j int) bool {
		return comments[i].ID < comments[j].ID
	})
	return comments, nil
}
