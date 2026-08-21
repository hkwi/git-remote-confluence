package confluence

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"

	"github.com/hkwi/git-remote-confluence/internal/confluencetypes"
)

type Attachment struct {
	ID         string                  `json:"id"`
	Title      string                  `json:"title"`
	Version    confluencetypes.Version `json:"version"`
	History    AttachmentHistory       `json:"history"`
	Extensions AttachmentExtensions    `json:"extensions"`
	Links      map[string]string       `json:"_links"`
}

type AttachmentHistory struct {
	PreviousVersion confluencetypes.Version `json:"previousVersion"`
}

type AttachmentExtensions struct {
	FileSize  int64  `json:"fileSize"`
	MediaType string `json:"mediaType"`
}

type attachmentListResponse struct {
	Results []Attachment      `json:"results"`
	Links   map[string]string `json:"_links"`
}

func (c *Client) FetchAttachment(attachmentID string) (Attachment, error) {
	if attachmentID == "" {
		return Attachment{}, fmt.Errorf("Confluence attachment id is required")
	}
	values := url.Values{}
	values.Set("expand", attachmentVersionExpand())
	var attachment Attachment
	if err := c.getJSON(c.apiPath("content/"+url.PathEscape(attachmentID)), values, &attachment); err != nil {
		return Attachment{}, err
	}
	if attachment.ID == "" {
		return Attachment{}, fmt.Errorf("Confluence attachment response is missing id")
	}
	return attachment, nil
}

func (c *Client) FetchAttachments(pageID string) ([]Attachment, error) {
	var attachments []Attachment
	start := 0
	const limit = 100

	for {
		values := url.Values{}
		values.Set("start", strconv.Itoa(start))
		values.Set("limit", strconv.Itoa(limit))
		values.Set("expand", attachmentVersionExpand())

		var response attachmentListResponse
		path := c.apiPath("content/" + url.PathEscape(pageID) + "/child/attachment")
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
	return c.DownloadAttachmentVersion(attachment, 0)
}

func (c *Client) FetchAttachmentVersions(attachment Attachment) ([]confluencetypes.Version, error) {
	if attachment.ID == "" {
		return nil, fmt.Errorf("Confluence attachment response is missing id")
	}
	if attachment.Version.Number <= 0 {
		current, err := c.FetchAttachment(attachment.ID)
		if err != nil {
			return nil, err
		}
		attachment = current
	}

	versions := []confluencetypes.Version{attachment.Version}
	seen := map[int]bool{attachment.Version.Number: true}
	previous := attachment.History.PreviousVersion
	for previous.Number > 0 {
		if seen[previous.Number] {
			return nil, fmt.Errorf("Confluence attachment %s version history contains a cycle at version %d", attachment.ID, previous.Number)
		}
		historical, err := c.fetchHistoricalAttachment(attachment.ID, previous.Number)
		if err != nil {
			return nil, fmt.Errorf("fetch historical attachment %s version %d: %w", attachment.ID, previous.Number, err)
		}
		if historical.Version.Number != previous.Number {
			return nil, fmt.Errorf("Confluence attachment %s requested version %d but returned version %d", attachment.ID, previous.Number, historical.Version.Number)
		}
		versions = append(versions, historical.Version)
		seen[historical.Version.Number] = true
		previous = historical.History.PreviousVersion
	}

	sort.Slice(versions, func(i, j int) bool { return versions[i].Number < versions[j].Number })
	return versions, nil
}

func (c *Client) fetchHistoricalAttachment(attachmentID string, version int) (Attachment, error) {
	values := url.Values{}
	values.Set("status", "historical")
	values.Set("version", strconv.Itoa(version))
	values.Set("expand", attachmentVersionExpand())
	var attachment Attachment
	if err := c.getJSON(c.apiPath("content/"+url.PathEscape(attachmentID)), values, &attachment); err != nil {
		return Attachment{}, err
	}
	if attachment.ID == "" {
		return Attachment{}, fmt.Errorf("Confluence historical attachment response is missing id")
	}
	return attachment, nil
}

func attachmentVersionExpand() string {
	return "version,version.by,history.previousVersion,history.previousVersion.by"
}

func (c *Client) DownloadAttachmentVersion(attachment Attachment, version int) ([]byte, error) {
	if attachment.ID == "" {
		return nil, fmt.Errorf("Confluence attachment response is missing id")
	}
	download := attachment.Links["download"]
	if download == "" {
		return nil, fmt.Errorf("Confluence attachment %s is missing download link", attachment.ID)
	}
	downloadURL := resolveLink(c.BaseURL, download)
	if version > 0 {
		parsed, err := url.Parse(downloadURL)
		if err != nil {
			return nil, fmt.Errorf("invalid Confluence attachment download link: %w", err)
		}
		query := parsed.Query()
		query.Set("version", strconv.Itoa(version))
		parsed.RawQuery = query.Encode()
		downloadURL = parsed.String()
	}
	return c.getBytes(downloadURL)
}
