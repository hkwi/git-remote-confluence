# git-remote-confluence

`git-remote-confluence` is a Git remote helper that treats a Confluence page
tree or space as a Git remote. It imports Confluence storage-format XML into a
Git repository and can push committed body updates for existing pages back to
Confluence.

The helper is intentionally narrow: Confluence remains the system that owns page
identity, hierarchy, version numbers, and storage-format XML. Git becomes the
place where page bodies and sync metadata can be reviewed, edited, and
committed.

## Sync Model

On fetch or clone, the helper resolves the remote URL to either a page root or a
space root, reads pages through the Confluence REST API, and writes a Git
fast-import stream. A page root imports that page and its descendants. A space
root imports the current pages in that space and reconstructs the hierarchy from
ancestor metadata.

On push, the helper scans committed page metadata, finds pages whose stored body
content changed, checks the Confluence version and storage hash, and updates the
existing Confluence page body. It refuses to overwrite a page when Confluence no
longer matches the imported metadata.

Create, delete, move, title, and attachment changes are not pushed yet.

## Working With Markdown

Confluence storage-format XML is the synchronized content format. The companion
`git-confluence` clean/smudge filter makes that practical for day-to-day
editing:

- Git stores each page body as Confluence storage-format XML.
- The working tree can show the same file as Markdown on checkout.
- `git add` can convert the edited Markdown back to Confluence storage XML.

That gives users Markdown editing while preserving the storage XML that
Confluence needs for reliable import and push.

The imported repository includes this `.gitattributes` entry:

```gitattributes
*.md filter=confluence-storage diff=markdown
```

Configure the `git-confluence` filter before checking out imported files.

## Repository Layout

Each Confluence page is represented by two files:

```text
<pageId>.md
<pageId>.yml
```

Child pages are placed under their parent's page-id directory:

```text
123456789.md
123456789.yml
123456789/123456790.md
123456789/123456790.yml
```

The `.md` file is stored in Git as Confluence storage-format XML. With the
`git-confluence` filter configured, it is checked out as Markdown and converted
back to storage XML on `git add`.

The `.yml` file contains page metadata, including the Confluence version number,
links, parent and child page IDs, file paths, and a SHA-256 hash of the stored
XML content for push conflict checks.

## Build

Build the helper and put it on `PATH`:

```sh
go build .
```

This writes `./git-remote-confluence`. If `git --exec-path` already contains an
older `git-remote-confluence`, replace that binary as well because Git may
prefer helpers from its exec path over `PATH`.

## Install

Install the tagged release with Go:

```sh
go install github.com/hkwi/git-remote-confluence@v0.1.0
```

Prebuilt archives for Linux, macOS, and Windows are published on the GitHub
Releases page. Each release includes `checksums.txt`.

Check the installed binary:

```sh
git-remote-confluence version
```

## Authentication

The helper needs a Confluence personal access token. It reads the first value it
finds from:

- `CONFLUENCE_PAT`
- `GIT_REMOTE_CONFLUENCE_PAT`
- `remote.<name>.pat`
- `confluence.pat`
- `remote.confluence.pat`

## Clone

If the `git-confluence` filter is already configured globally, clone with Git's
explicit remote-helper syntax:

```sh
CONFLUENCE_PAT=... git clone \
  'confluence::https://confluence.example.com/pages/viewpage.action?pageId=123456789'
```

For a per-clone filter configuration, clone without checkout, configure the
filter, then check out:

```sh
CONFLUENCE_PAT=... git clone --no-checkout \
  'confluence::https://confluence.example.com/pages/viewpage.action?pageId=123456789' \
  pages
cd pages
git config filter.confluence-storage.clean "/path/to/git-confluence/git-confluence clean"
git config filter.confluence-storage.smudge "/path/to/git-confluence/git-confluence smudge"
git config filter.confluence-storage.required true
git checkout
```

The remote URL may identify a page by `pageId`, a display page URL, or a
Confluence space.

## Push

After editing and committing page Markdown, push existing page body updates back
to Confluence:

```sh
git push origin HEAD:main
git fetch origin
```

Fetch after a successful push to refresh the Confluence page version and local
metadata.

## Configured Remote

For a configured remote, `confluence:https://...` is accepted by the helper when
Git is told to use the `confluence` VCS helper:

```ini
[remote "origin"]
  vcs = confluence
  url = confluence:https://confluence.example.com/pages/viewpage.action?pageId=123456789
  pat = somevalue
```

## Progress

To show helper progress, ask Git for progress or verbose output:

```sh
CONFLUENCE_PAT=... git clone --progress --verbose \
  'confluence::https://confluence.example.com/pages/viewpage.action?pageId=123456789'
```

Progress is written to stderr. When Git captures helper stderr during a
successful import, the helper also mirrors progress to the controlling terminal
if one is available.

## Tests

```sh
go test ./...
```
