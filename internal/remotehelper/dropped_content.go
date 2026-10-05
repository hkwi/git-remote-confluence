package remotehelper

import (
	"sort"
	"strconv"
	"strings"

	"github.com/hkwi/git-remote-confluence/internal/confluence"
	"github.com/hkwi/git-remote-confluence/internal/gitrepo"
)

const droppedPathSampleSize = 10

// reportDroppedContent warns when unavailable content removes pages or
// attachments that a previous import had already committed. The import still
// proceeds: the earlier commit keeps the content, so the loss must at least be
// visible rather than silent.
func (h *helper) reportDroppedContent(parent string, result confluence.FetchResult) {
	incomplete := len(result.SkippedPages) + len(result.UnavailableChildLists) + len(result.UnavailableAttachments)
	if parent == "" || incomplete == 0 {
		return
	}
	previous, err := gitrepo.ListTree(parent)
	if err != nil {
		h.reportWarning("cannot compare this import against the previous one (%s): %v", parent, err)
		return
	}

	imported := map[string]bool{}
	attachmentDirs := map[string]string{}
	for _, page := range result.Pages {
		imported[page.ContentPath()] = true
		for _, attachment := range page.Attachments {
			imported[attachment.Path] = true
		}
		if page.AttachmentsErrorStatus != 0 {
			attachmentDirs[page.PageID] = page.AttachmentsDir() + "/"
		}
	}

	var droppedPages []string
	for path := range previous {
		if strings.HasSuffix(path, ".md") && !imported[path] {
			droppedPages = append(droppedPages, path)
		}
	}
	if len(droppedPages) > 0 {
		h.reportWarning("%d pages kept by the previous import are missing from this import: %s; the import continues and the earlier commit retains them",
			len(droppedPages), summarizePaths(droppedPages))
	}

	for _, failure := range result.UnavailableAttachments {
		directory, ok := attachmentDirs[failure.PageID]
		if !ok {
			continue
		}
		var dropped []string
		for path := range previous {
			if strings.HasPrefix(path, directory) && !imported[path] {
				dropped = append(dropped, path)
			}
		}
		if len(dropped) == 0 {
			continue
		}
		h.reportWarning("page %s: HTTP %d dropped %d attachment(s) kept by the previous import: %s; the import continues and the earlier commit retains them",
			failure.PageID, failure.StatusCode, len(dropped), summarizePaths(dropped))
	}
}

func summarizePaths(paths []string) string {
	sort.Strings(paths)
	if len(paths) <= droppedPathSampleSize {
		return strings.Join(paths, ", ")
	}
	return strings.Join(paths[:droppedPathSampleSize], ", ") +
		", and " + strconv.Itoa(len(paths)-droppedPathSampleSize) + " more"
}
