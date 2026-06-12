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
