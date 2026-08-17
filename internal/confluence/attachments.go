package confluence

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
)

type Attachment struct {
	ID    string            `json:"id"`
	Title string            `json:"title"`
	Links map[string]string `json:"_links"`
}

type attachmentListResponse struct {
	Results []Attachment      `json:"results"`
	Links   map[string]string `json:"_links"`
}

func (c *Client) FetchAttachments(pageID string) ([]Attachment, error) {
	var attachments []Attachment
	start := 0
	const limit = 100

	for {
		values := url.Values{}
		values.Set("start", strconv.Itoa(start))
		values.Set("limit", strconv.Itoa(limit))

		var response attachmentListResponse
		path := "/rest/api/content/" + url.PathEscape(pageID) + "/child/attachment"
		if err := c.getJSON(path, values, &response); err != nil {
			return nil, err
		}
		attachments = append(attachments, response.Results...)
		if len(response.Results) == 0 || len(response.Results) < limit || response.Links["next"] == "" {
			break
		}
		start += len(response.Results)
	}

	sort.Slice(attachments, func(i, j int) bool {
		if attachments[i].Title == attachments[j].Title {
			return attachments[i].ID < attachments[j].ID
		}
		return attachments[i].Title < attachments[j].Title
	})
	return attachments, nil
}

func (c *Client) DownloadAttachment(attachment Attachment) ([]byte, error) {
	if attachment.ID == "" {
		return nil, fmt.Errorf("Confluence attachment response is missing id")
	}
	download := attachment.Links["download"]
	if download == "" {
		return nil, fmt.Errorf("Confluence attachment %s is missing download link", attachment.ID)
	}
	return c.getBytes(resolveLink(c.BaseURL, download))
}
