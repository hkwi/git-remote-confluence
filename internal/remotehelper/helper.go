package remotehelper

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"git-remote-confluence/internal/confluence"
	"git-remote-confluence/internal/fastimport"
)

func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	remoteName, remoteURL, err := remoteArgs(args)
	if err != nil {
		return err
	}
	progressOut, closeProgress := progressWriter(stderr)
	defer closeProgress()

	return (&helper{
		remoteName: remoteName,
		remoteURL:  remoteURL,
		in:         bufio.NewReader(stdin),
		out:        stdout,
		err:        progressOut,
		verbosity:  1,
		progress:   true,
	}).serve()
}

type helper struct {
	remoteName string
	remoteURL  string
	in         *bufio.Reader
	out        io.Writer
	err        io.Writer
	verbosity  int
	progress   bool
}

func (h *helper) serve() error {
	for {
		line, err := h.readLine()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if line == "" {
			return nil
		}

		switch {
		case line == "capabilities":
			if _, err := io.WriteString(h.out, "import\npush\noption\nrefspec refs/heads/*:refs/heads/*\n\n"); err != nil {
				return err
			}
		case line == "list":
			if _, err := fmt.Fprintf(h.out, "? %s\n@%s HEAD\n\n", fastimport.DefaultBranch, fastimport.DefaultBranch); err != nil {
				return err
			}
		case line == "list for-push":
			if _, err := io.WriteString(h.out, "\n"); err != nil {
				return err
			}
		case strings.HasPrefix(line, "option "):
			if err := h.handleOption(line); err != nil {
				return err
			}
		case strings.HasPrefix(line, "import "):
			refs, err := h.readImportBatch(line)
			if err != nil {
				return err
			}
			return h.runImport(refs)
		case strings.HasPrefix(line, "push "):
			pushes, err := h.readPushBatch(line)
			if err != nil {
				return err
			}
			return h.runPushBatch(pushes)
		default:
			return fmt.Errorf("unsupported remote-helper command: %s", line)
		}
	}
}

func (h *helper) handleOption(line string) error {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 {
		_, err := io.WriteString(h.out, "unsupported\n")
		return err
	}

	name, value := parts[1], parts[2]
	switch name {
	case "verbosity":
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
			_, writeErr := io.WriteString(h.out, "error invalid verbosity\n")
			if writeErr != nil {
				return writeErr
			}
			return nil
		}
		h.verbosity = parsed
		_, err := io.WriteString(h.out, "ok\n")
		return err
	case "progress":
		h.progress = value == "true"
		_, err := io.WriteString(h.out, "ok\n")
		return err
	case "cloning", "check-connectivity":
		_, err := io.WriteString(h.out, "ok\n")
		return err
	default:
		_, err := io.WriteString(h.out, "unsupported\n")
		return err
	}
}

func (h *helper) readImportBatch(first string) ([]string, error) {
	refs := []string{strings.TrimPrefix(first, "import ")}
	for {
		line, err := h.readLine()
		if err == io.EOF || line == "" {
			return refs, nil
		}
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(line, "import ") {
			return nil, fmt.Errorf("unexpected command in import batch: %s", line)
		}
		refs = append(refs, strings.TrimPrefix(line, "import "))
	}
}

func (h *helper) runImport(refs []string) error {
	location, client, err := h.confluenceClient()
	if err != nil {
		return err
	}
	location, err = confluence.ResolveLocation(client, location, h.reportProgress)
	if err != nil {
		return err
	}
	h.reportProgress("root %s %s at %s", location.RootType, location.RootValue, location.BaseURL)

	pages, err := confluence.FetchPagesWithProgress(client, location, h.reportProgress)
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		return fmt.Errorf("Confluence returned no pages")
	}
	h.reportProgress("fetched %d Confluence pages", len(pages))

	stream := fastimport.BuildStreamWithProgress(
		fastimport.SelectBranch(refs),
		fastimport.Location{RootType: location.RootType, RootValue: location.RootValue},
		pages,
		h.showProgress(),
	)
	h.reportProgress("writing %d bytes to git fast-import", len(stream))
	_, err = h.out.Write(stream)
	if err == nil {
		h.reportProgress("done")
	}
	return err
}

func (h *helper) confluenceClient() (confluence.Location, *confluence.Client, error) {
	location, err := confluence.ParseLocation(h.remoteURL)
	if err != nil {
		return confluence.Location{}, nil, err
	}

	pat := confluence.ResolvePAT(h.remoteName)
	if pat == "" {
		return confluence.Location{}, nil, fmt.Errorf("Confluence PAT is required; set CONFLUENCE_PAT or remote.%s.pat", h.remoteName)
	}

	return location, confluence.NewClient(location.BaseURL, pat), nil
}

func (h *helper) reportProgress(format string, args ...any) {
	if h.err == nil || !h.showProgress() {
		return
	}
	fmt.Fprintf(h.err, "confluence: "+format+"\n", args...)
}

func (h *helper) showProgress() bool {
	return h.progress && h.verbosity > 0
}

func (h *helper) readLine() (string, error) {
	line, err := h.in.ReadString('\n')
	if err != nil {
		if err == io.EOF && line != "" {
			return strings.TrimRight(line, "\n"), nil
		}
		return "", err
	}
	return strings.TrimRight(line, "\n"), nil
}

func remoteArgs(args []string) (string, string, error) {
	if len(args) == 0 {
		return "", "", fmt.Errorf("missing remote argument")
	}
	remoteName := args[0]
	remoteURL := args[0]
	if len(args) > 1 {
		remoteURL = args[1]
	}
	return remoteName, remoteURL, nil
}

func progressWriter(stderr io.Writer) (io.Writer, func()) {
	file, ok := stderr.(*os.File)
	if !ok || isCharDevice(file) {
		return stderr, func() {}
	}

	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return stderr, func() {}
	}
	return io.MultiWriter(stderr, tty), func() { _ = tty.Close() }
}

func isCharDevice(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
