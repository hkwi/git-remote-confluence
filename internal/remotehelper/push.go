package remotehelper

import (
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hkwi/git-remote-confluence/internal/confluence"
	"github.com/hkwi/git-remote-confluence/internal/gitrepo"
	"github.com/hkwi/git-remote-confluence/internal/pagefiles"
)

type pushRef struct {
	src   string
	dst   string
	force bool
}

type pageUpdate struct {
	metadata   pagefiles.Metadata
	storageXML string
	path       string
}

func (h *helper) readPushBatch(first string) ([]pushRef, error) {
	pushes := []pushRef{}
	if push, ok, err := parsePushLine(first); err != nil {
		return nil, err
	} else if ok {
		pushes = append(pushes, push)
	}

	for {
		line, err := h.readLine()
		if err == io.EOF || line == "" {
			return pushes, nil
		}
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(line, "option ") {
			continue
		}
		push, ok, err := parsePushLine(line)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("unexpected command in push batch: %s", line)
		}
		pushes = append(pushes, push)
	}
}

func parsePushLine(line string) (pushRef, bool, error) {
	if !strings.HasPrefix(line, "push ") {
		return pushRef{}, false, nil
	}
	spec := strings.TrimSpace(strings.TrimPrefix(line, "push "))
	force := strings.HasPrefix(spec, "+")
	spec = strings.TrimPrefix(spec, "+")
	src, dst, ok := strings.Cut(spec, ":")
	if !ok || dst == "" {
		return pushRef{}, false, fmt.Errorf("invalid push refspec: %s", spec)
	}
	return pushRef{src: src, dst: dst, force: force}, true, nil
}

func (h *helper) runPushBatch(pushes []pushRef) error {
	for _, push := range pushes {
		if push.src == "" {
			if err := h.writePushStatus("error", push.dst, "deleting Confluence pages is not supported"); err != nil {
				return err
			}
			continue
		}

		updated, err := h.pushRef(push.src)
		if err != nil {
			if writeErr := h.writePushStatus("error", push.dst, sanitizeStatus(err.Error())); writeErr != nil {
				return writeErr
			}
			continue
		}
		h.reportProgress("updated %d Confluence pages from %s", updated, push.src)
		if err := h.writePushStatus("ok", push.dst, ""); err != nil {
			return err
		}
	}
	_, err := io.WriteString(h.out, "\n")
	return err
}

func (h *helper) writePushStatus(status, dst, reason string) error {
	if reason == "" {
		_, err := fmt.Fprintf(h.out, "%s %s\n", status, dst)
		return err
	}
	_, err := fmt.Fprintf(h.out, "%s %s %s\n", status, dst, reason)
	return err
}

func sanitizeStatus(value string) string {
	value = strings.NewReplacer("\n", " ", "\r", " ").Replace(value)
	if value == "" {
		return "push failed"
	}
	return value
}

func (h *helper) pushRef(src string) (int, error) {
	_, client, err := h.confluenceClient()
	if err != nil {
		return 0, err
	}

	updates, err := buildPageUpdates(src)
	if err != nil {
		return 0, err
	}
	if len(updates) == 0 {
		h.reportProgress("no Confluence page changes in %s", src)
		return 0, nil
	}

	for _, update := range updates {
		if err := h.updatePage(client, update); err != nil {
			return 0, err
		}
	}
	return len(updates), nil
}

func buildPageUpdates(src string) ([]pageUpdate, error) {
	tree, err := gitrepo.ListTree(src)
	if err != nil {
		return nil, err
	}

	var metadataPaths []string
	for path := range tree {
		if strings.HasSuffix(path, ".yml") {
			metadataPaths = append(metadataPaths, path)
		}
	}
	sort.Strings(metadataPaths)

	var updates []pageUpdate
	for _, metadataPath := range metadataPaths {
		metadataBlob := tree[metadataPath]
		metadataData, err := gitrepo.CatBlob(metadataBlob.OID)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", metadataPath, err)
		}
		metadata, err := pagefiles.ParseMetadataYAML(metadataData)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", metadataPath, err)
		}
		storagePath := cleanGitPath(metadata.StoragePath)
		storageBlob, ok := tree[storagePath]
		if !ok {
			return nil, fmt.Errorf("metadata %s refers to missing storage file %s", metadataPath, storagePath)
		}
		storageData, err := gitrepo.CatBlob(storageBlob.OID)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", storagePath, err)
		}

		hash := sha256.Sum256(storageData)
		currentHash := fmt.Sprintf("%x", hash[:])
		if currentHash == metadata.StorageSHA256 {
			continue
		}
		updates = append(updates, pageUpdate{
			metadata:   metadata,
			storageXML: string(storageData),
			path:       storagePath,
		})
	}
	return updates, nil
}

func cleanGitPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func (h *helper) updatePage(client *confluence.Client, update pageUpdate) error {
	page, err := client.FetchPage(update.metadata.PageID)
	if err != nil {
		return err
	}
	if page.Version.Number != update.metadata.VersionNumber {
		return fmt.Errorf(
			"page %s version conflict: metadata has %d, Confluence has %d; fetch before pushing",
			update.metadata.PageID,
			update.metadata.VersionNumber,
			page.Version.Number,
		)
	}

	currentHash := sha256.Sum256([]byte(page.Body.Storage.Value))
	if fmt.Sprintf("%x", currentHash[:]) != update.metadata.StorageSHA256 {
		return fmt.Errorf("page %s content conflict: Confluence content differs from metadata hash; fetch before pushing", update.metadata.PageID)
	}

	title := page.Title
	if title == "" {
		title = update.metadata.Title
	}
	spaceKey := page.Space.Key
	if spaceKey == "" {
		spaceKey = update.metadata.SpaceKey
	}

	h.reportProgress("updating Confluence page %s from %s", update.metadata.PageID, update.path)
	return client.UpdatePage(confluence.PageUpdate{
		ID:            update.metadata.PageID,
		Title:         title,
		SpaceKey:      spaceKey,
		StorageXML:    update.storageXML,
		VersionNumber: page.Version.Number + 1,
		Message:       "Update from git-remote-confluence",
	})
}
