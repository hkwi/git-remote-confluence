package remotehelper

import (
	"sort"
	"strings"

	"github.com/hkwi/git-remote-confluence/internal/confluence"
	"github.com/hkwi/git-remote-confluence/internal/gitrepo"
)

// reportDroppedAttachments warns when refused attachment permissions remove
// attachments that a previous import had already committed. The import still
// proceeds: the earlier commit keeps the content, so the loss must at least be
// visible rather than silent.
func (h *helper) reportDroppedAttachments(parent string, result confluence.FetchResult) {
	if parent == "" || len(result.UnavailableAttachments) == 0 {
		return
	}
	previous, err := gitrepo.ListTree(parent)
	if err != nil {
		h.reportWarning("cannot compare attachments against the previous import %s: %v", parent, err)
		return
	}

	imported := map[string]bool{}
	directories := map[string]string{}
	for _, page := range result.Pages {
		for _, attachment := range page.Attachments {
			imported[attachment.Path] = true
		}
		if page.AttachmentsErrorStatus != 0 {
			directories[page.PageID] = page.AttachmentsDir() + "/"
		}
	}

	for _, failure := range result.UnavailableAttachments {
		directory, ok := directories[failure.PageID]
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
		sort.Strings(dropped)
		h.reportWarning("page %s: HTTP %d dropped %d attachment(s) kept by the previous import: %s; the import continues and the earlier commit retains them",
			failure.PageID, failure.StatusCode, len(dropped), strings.Join(dropped, ", "))
	}
}
