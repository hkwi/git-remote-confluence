package confluence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/hkwi/git-remote-confluence/internal/confluencetypes"
)

const appName = "git-remote-confluence"

var userAgentVersion = "dev"

func SetUserAgentVersion(version string) {
	if version != "" {
		userAgentVersion = version
	}
}

func userAgent() string {
	return appName + "/" + userAgentVersion
}

func clientUserAgent(c *Client) string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return userAgent()
}

type Client struct {
	BaseURL    string
	PAT        string
	HTTPClient *http.Client
	UserAgent  string
}

type Page struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Status    string            `json:"status"`
	Title     string            `json:"title"`
	Space     Space             `json:"space"`
	Version   Version           `json:"version"`
	Ancestors []Ancestor        `json:"ancestors"`
	Body      Body              `json:"body"`
	Links     map[string]string `json:"_links"`
}

type Space struct {
	Key string `json:"key"`
}

type Version = confluencetypes.Version

type User = confluencetypes.User

type Ancestor struct {
	ID string `json:"id"`
}

type Body struct {
	Storage Storage `json:"storage"`
}

type Storage struct {
	Value string `json:"value"`
}

type listResponse struct {
	Results []Page            `json:"results"`
	Size    int               `json:"size"`
	Limit   int               `json:"limit"`
	Links   map[string]string `json:"_links"`
}

type PageUpdate struct {
	ID            string
	Title         string
	SpaceKey      string
	StorageXML    string
	VersionNumber int
	Message       string
	MinorEdit     bool
}

func NewClient(baseURL, pat string) *Client {
	return &Client{
		BaseURL: baseURL,
		PAT:     pat,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) FetchPage(pageID string) (Page, error) {
	var page Page
	err := c.getJSON("/rest/api/content/"+url.PathEscape(pageID), commonExpand(), &page)
	if err != nil {
		return Page{}, err
	}
	if page.ID == "" {
		return Page{}, fmt.Errorf("Confluence page response is missing id")
	}
	return page, nil
}

func (c *Client) FetchChildren(pageID string) ([]Page, error) {
	return c.paginated("/rest/api/content/"+url.PathEscape(pageID)+"/child/page", commonExpand())
}

func (c *Client) FetchPageByTitle(spaceKey, title string) (Page, error) {
	values := commonExpand()
	values.Set("spaceKey", spaceKey)
	values.Set("title", title)
	values.Set("type", "page")
	values.Set("status", "current")

	pages, err := c.paginated("/rest/api/content", values)
	if err != nil {
		return Page{}, err
	}
	if len(pages) == 0 {
		return Page{}, fmt.Errorf("Confluence page %q in space %s was not found", title, spaceKey)
	}
	if len(pages) > 1 {
		return Page{}, fmt.Errorf("Confluence page %q in space %s matched %d pages", title, spaceKey, len(pages))
	}
	if pages[0].ID == "" {
		return Page{}, fmt.Errorf("Confluence page %q in space %s response is missing id", title, spaceKey)
	}
	return pages[0], nil
}

func (c *Client) FetchSpacePages(spaceKey string) ([]Page, error) {
	values := commonExpand()
	values.Set("spaceKey", spaceKey)
	values.Set("type", "page")
	values.Set("status", "current")
	return c.paginated("/rest/api/content", values)
}

func (c *Client) UpdatePage(update PageUpdate) error {
	if update.ID == "" {
		return fmt.Errorf("Confluence page update is missing id")
	}
	if update.Title == "" {
		return fmt.Errorf("Confluence page update %s is missing title", update.ID)
	}
	if update.VersionNumber <= 0 {
		return fmt.Errorf("Confluence page update %s is missing version number", update.ID)
	}

	payload := map[string]any{
		"id":    update.ID,
		"type":  "page",
		"title": update.Title,
		"version": map[string]any{
			"number":    update.VersionNumber,
			"minorEdit": update.MinorEdit,
			"message":   update.Message,
		},
		"body": map[string]any{
			"storage": map[string]any{
				"value":          update.StorageXML,
				"representation": "storage",
			},
		},
	}
	if update.SpaceKey != "" {
		payload["space"] = map[string]any{"key": update.SpaceKey}
	}

	var page Page
	return c.putJSON("/rest/api/content/"+url.PathEscape(update.ID), payload, &page)
}

func (c *Client) paginated(path string, baseValues url.Values) ([]Page, error) {
	var pages []Page
	start := 0
	const limit = 100

	for {
		values := cloneValues(baseValues)
		values.Set("start", strconv.Itoa(start))
		values.Set("limit", strconv.Itoa(limit))

		var response listResponse
		if err := c.getJSON(path, values, &response); err != nil {
			return nil, err
		}
		pages = append(pages, response.Results...)

		if len(response.Results) == 0 || len(response.Results) < limit || response.Links["next"] == "" {
			break
		}
		start += len(response.Results)
	}
	return pages, nil
}

func (c *Client) getJSON(path string, values url.Values, target any) error {
	requestURL := c.BaseURL + path
	if encoded := values.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.PAT)
	req.Header.Set("User-Agent", clientUserAgent(c))

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Confluence API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Confluence API HTTP %d: %s", resp.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("Confluence API returned invalid JSON from %s: %w", requestURL, err)
	}
	return nil
}

func (c *Client) putJSON(path string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	requestURL := c.BaseURL + path
	req, err := http.NewRequest(http.MethodPut, requestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.PAT)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", clientUserAgent(c))

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Confluence API request failed: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Confluence API HTTP %d: %s", resp.StatusCode, string(responseBody))
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(responseBody, target); err != nil {
		return fmt.Errorf("Confluence API returned invalid JSON from %s: %w", requestURL, err)
	}
	return nil
}

func commonExpand() url.Values {
	values := url.Values{}
	values.Set("expand", "body.storage,version,version.by,ancestors,space,history.lastUpdated,metadata.labels")
	return values
}

func cloneValues(values url.Values) url.Values {
	cloned := url.Values{}
	for key, items := range values {
		copied := make([]string, len(items))
		copy(copied, items)
		cloned[key] = copied
	}
	return cloned
}
