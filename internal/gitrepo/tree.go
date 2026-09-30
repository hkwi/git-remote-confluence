package gitrepo

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Blob struct {
	Mode string
	Type string
	OID  string
	Path string
}

func ListTree(ref string) (map[string]Blob, error) {
	output, err := gitOutput("ls-tree", "-r", "-z", ref)
	if err != nil {
		return nil, err
	}

	blobs := map[string]Blob{}
	for _, entry := range bytes.Split(output, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		header, path, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok {
			return nil, fmt.Errorf("git ls-tree returned malformed entry %q", entry)
		}
		fields := strings.Fields(string(header))
		if len(fields) != 3 {
			return nil, fmt.Errorf("git ls-tree returned malformed header %q", header)
		}
		if fields[1] != "blob" {
			continue
		}
		blob := Blob{
			Mode: fields[0],
			Type: fields[1],
			OID:  fields[2],
			Path: string(path),
		}
		blobs[blob.Path] = blob
	}
	return blobs, nil
}

func CatBlob(oid string) ([]byte, error) {
	return gitOutput("cat-file", "blob", oid)
}

func gitOutput(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, output)
	}
	return output, nil
}

// ResolveCommit returns the full commit object ID that ref currently points
// to, or "" when the ref does not yet exist in the local repository. It is
// used to chain each Confluence import onto the previous local tip so that
// repeated fetches fast-forward instead of being rejected as rewriting
// history. A missing ref (the initial clone) is not an error.
func ResolveCommit(ref string) (string, error) {
	output, err := gitOutput("rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(output)), nil
}
