package confluence

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type Location struct {
	BaseURL     string
	RootType    string
	RootValue   string
	SpaceKey    string
	PageTitle   string
	OriginalURL string
}

func ParseLocation(rawURL string) (Location, error) {
	original := rawURL
	stripped := stripTransportPrefix(rawURL)

	parsed, err := url.Parse(stripped)
	if err != nil {
		return Location{}, err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Location{}, fmt.Errorf("remote URL must resolve to an http(s) Confluence URL")
	}

	query := parsed.Query()
	if pageID := firstQuery(query, "pageId", "pageid", "id"); pageID != "" && isDigits(pageID) {
		return Location{BaseURL: baseURL(parsed), RootType: "page", RootValue: pageID, OriginalURL: original}, nil
	}

	parts := pathParts(parsed.Path)
	if pageID := pageIDFromPath(parts); pageID != "" {
		return Location{BaseURL: baseURL(parsed), RootType: "page", RootValue: pageID, OriginalURL: original}, nil
	}

	if spaceKey, pageTitle := displayPageFromPath(parsed.EscapedPath()); spaceKey != "" && pageTitle != "" {
		return Location{
			BaseURL:     baseURL(parsed),
			RootType:    "page",
			SpaceKey:    spaceKey,
			PageTitle:   pageTitle,
			OriginalURL: original,
		}, nil
	}

	spaceKey := firstQuery(query, "spaceKey", "spacekey", "key")
	if spaceKey == "" {
		spaceKey = spaceKeyFromPath(parts)
	}
	if spaceKey != "" {
		return Location{BaseURL: baseURL(parsed), RootType: "space", RootValue: spaceKey, OriginalURL: original}, nil
	}

	return Location{}, fmt.Errorf("remote URL must identify a pageId or a Confluence space")
}

func ResolvePAT(remoteName string) string {
	for _, name := range []string{"CONFLUENCE_PAT", "GIT_REMOTE_CONFLUENCE_PAT"} {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}

	var keys []string
	if regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(remoteName) {
		keys = append(keys, "remote."+remoteName+".pat")
	}
	keys = append(keys, "confluence.pat", "remote.confluence.pat")

	for _, key := range keys {
		if value := gitConfigGet(key); value != "" {
			return value
		}
	}
	return ""
}

func stripTransportPrefix(rawURL string) string {
	switch {
	case strings.HasPrefix(rawURL, "confluence::"):
		return strings.TrimPrefix(rawURL, "confluence::")
	case strings.HasPrefix(rawURL, "confluence://"):
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return rawURL
		}
		parsed.Scheme = "https"
		return parsed.String()
	case strings.HasPrefix(rawURL, "confluence:"):
		return strings.TrimPrefix(rawURL, "confluence:")
	default:
		return rawURL
	}
}

func baseURL(parsed *url.URL) string {
	path := parsed.Path
	basePath := path
	for _, marker := range []string{"/rest/", "/pages/", "/display/", "/spaces/"} {
		if index := strings.Index(basePath, marker); index >= 0 {
			basePath = basePath[:index]
		}
	}
	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: strings.TrimRight(basePath, "/")}).String()
}

func firstQuery(query url.Values, names ...string) string {
	for _, name := range names {
		for key, values := range query {
			if strings.EqualFold(key, name) && len(values) > 0 {
				return values[0]
			}
		}
	}
	return ""
}

func pathParts(path string) []string {
	var parts []string
	for _, part := range strings.Split(path, "/") {
		if part == "" {
			continue
		}
		unescaped, err := url.PathUnescape(part)
		if err != nil {
			parts = append(parts, part)
			continue
		}
		parts = append(parts, unescaped)
	}
	return parts
}

func pageIDFromPath(parts []string) string {
	for index := 0; index+1 < len(parts); index++ {
		if parts[index] == "pages" && isDigits(parts[index+1]) {
			return parts[index+1]
		}
	}
	return ""
}

func spaceKeyFromPath(parts []string) string {
	for _, marker := range []string{"spaces", "display"} {
		for index, part := range parts {
			if part == marker && index+1 < len(parts) {
				return parts[index+1]
			}
		}
	}
	return ""
}

func displayPageFromPath(escapedPath string) (string, string) {
	parts := escapedPathParts(escapedPath)
	for index, part := range parts {
		if part != "display" || index+2 >= len(parts) {
			continue
		}
		spaceKey, err := url.PathUnescape(parts[index+1])
		if err != nil {
			return "", ""
		}
		title, err := displayTitleFromEscapedPart(parts[index+2])
		if err != nil {
			return "", ""
		}
		return spaceKey, title
	}
	return "", ""
}

func escapedPathParts(path string) []string {
	var parts []string
	for _, part := range strings.Split(path, "/") {
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

func displayTitleFromEscapedPart(part string) (string, error) {
	return url.PathUnescape(strings.ReplaceAll(part, "+", "%20"))
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}

func gitConfigGet(key string) string {
	cmd := exec.Command("git", "config", "--get", key)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
