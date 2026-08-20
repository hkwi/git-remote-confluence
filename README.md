# git-remote-confluence

`git-remote-confluence` is a Git remote helper that treats a Confluence page
tree or space as a Git remote. It imports Confluence storage-format XML and
attachment metadata pointers into a Git repository. The optional
`git-confluence` filter materializes page Markdown and attachment bytes in the
working tree. The helper can push committed body updates for existing pages
back to Confluence.

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

Git stores each attachment as a small text pointer containing its Confluence
identity and current version. Clone and fetch therefore do not download
attachment bytes. With the `git-confluence` filter configured, checkout
downloads the version named by the pointer and exposes it as a normal working
file. Create, delete, move, title, and attachment changes are not pushed yet.

## Working With Markdown

Confluence storage-format XML is the synchronized content format. The companion
`git-confluence` filter makes pages and attachments practical for day-to-day
use:

- Git stores each page body as Confluence storage-format XML.
- The working tree can show the same file as Markdown on checkout.
- `git add` can convert the edited Markdown back to Confluence storage XML.
- Git stores attachment pointers and can materialize their bytes on checkout.

That gives users Markdown editing while preserving the storage XML that
Confluence needs for reliable import and push.

The imported repository includes this `.gitattributes` entry:

```gitattributes
*.md filter=confluence diff=markdown
**/attachments/** filter=confluence -text
```

Without the filter, checkout leaves storage XML and attachment pointers intact.
This is the lightweight default for agents and offline use. Configure the
filter once with `git confluence install --global` to expose Markdown and
attachment bytes for normal human use.

The `install` subcommand registers the unified clean/smudge driver in Git
configuration. It does not install the executable or download attachments.

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

Attachments use their normal filenames below the page ID that owns them:

```text
123456789/attachments/diagram.png
123456789/123456790/attachments/notes.pdf
```

Path separators and control characters in attachment names are replaced with
underscores so an attachment cannot escape its page's `attachments` directory.
Git stores a canonical text pointer at each attachment path. The pointer records
the source site, page ID, attachment ID, version, filename, size, media type,
and stable download path. It contains no token and no attachment bytes. The
configured filter replaces the pointer with that version's bytes in the working
tree and restores the same pointer on `git add`. Locally modified attachment
bytes are rejected because attachment push is not supported yet.

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

For human-oriented working trees, configure the unified page and attachment
filter once, then clone with Git's explicit remote-helper syntax:

```sh
git confluence install --global
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
git confluence install --local
git checkout
```

To keep attachment pointers during checkout while still converting pages, set
the skip-smudge variable for that command:

```sh
GIT_CONFLUENCE_SKIP_SMUDGE=1 git checkout
```

Afterward, materialize all or selected attachments explicitly:

```sh
git confluence pull
git confluence pull 123456789/attachments/diagram.png
```

The remote URL may identify a page by `pageId`, a display page URL, or a
Confluence space. A URL containing both `pageId` and `attachmentId` can still be
cloned directly when a standalone repository containing the attachment's full
binary version history is required.

For Confluence Data Center, attachment history is read by following
`history.previousVersion` through historical content responses. The helper does
not require a `GET /content/{id}/version` listing endpoint.

## REST API Path

By default, REST requests use the traditional unversioned `/rest/api` root. If
a Confluence installation exposes the API below a different root or requires an
explicit version, set either or both of these variables when cloning:

```sh
CONFLUENCE_API_ROOT=custom/api/root \
CONFLUENCE_API_VERSION=2.0 \
CONFLUENCE_PAT=... \
git clone 'confluence::https://confluence.example.com/pages/viewpage.action?pageId=123456789'
```

This example sends content requests below
`/custom/api/root/2.0/content/...`. `CONFLUENCE_API_VERSION` is omitted by
default, preserving `/rest/api/content/...`. The aliases
`GIT_REMOTE_CONFLUENCE_API_ROOT` and `GIT_REMOTE_CONFLUENCE_API_VERSION` are
also accepted.

For an existing configured remote, use Git configuration instead:

```ini
[remote "origin"]
  apiRoot = custom/api/root
  apiVersion = 2.0
```

The fallback keys are `confluence.apiRoot`, `confluence.apiVersion`,
`remote.confluence.apiRoot`, and `remote.confluence.apiVersion`. Download and
browser links returned by Confluence remain relative to the site URL; the API
root and version are not added to them.

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
