# Publishing a goblog plugin

The directory at [goblog.live/plugins](https://goblog.live/plugins) lists plugins from this registry. A plugin is a GitHub repository; each GitHub release is a version. Submitting means adding your repository to `registry.yaml` in a pull request — CI validates it and, once merged, the index is rebuilt (on every merge and every six hours).

## What the repository must contain

At the root of the repository, at the release tag being published (the tool checks the latest release):

| File | Required | Notes |
|---|---|---|
| `goblog-plugin.json` | yes | the manifest, below |
| the entry file (default `plugin.go`) | yes | a [dynamic plugin](https://github.com/goblogplatform/goblog#dynamic-plugins): `package main`, `func NewPlugin() plugin.Plugin` |
| `README.md` | yes | shown on the plugin's directory page |
| `CHANGELOG.md` | no | shown when present |
| `LICENSE` | recommended | should match `license` in the manifest |

### `goblog-plugin.json`

```json
{
  "name": "hello",
  "display_name": "Hello",
  "description": "One sentence shown in the listing.",
  "author": "Your Name",
  "license": "Apache-2.0",
  "entry": "plugin.go",
  "min_goblog_version": "0.2.6",
  "homepage": "https://example.com/optional"
}
```

- `name`: `^[a-z0-9-]+$`, unique across the registry, and equal to what your plugin's `Name()` returns.
- `display_name`: the label shown in the directory. It does not have to equal your plugin's `DisplayName()`, which labels its settings group in the admin UI.
- `license`: an SPDX identifier from the list in `internal/registry/manifest.go` (MIT, Apache-2.0, BSD-2/3-Clause, ISC, MPL-2.0, GPL/LGPL/AGPL `-only`/`-or-later`, Unlicense, 0BSD). Open an issue to add another.
- `entry`: a `.go` file at the repository root; defaults to `plugin.go`.
- `min_goblog_version`: plain semver (`0.2.6`, no `v`) — the oldest goblog your plugin works with.

### Releases

- Tag releases `vX.Y.Z` (exactly three numbers). Drafts and pre-releases are ignored.
- The tag without `v` must equal the string your plugin's `Version()` returns.
- The GitHub release body is shown as the version's release notes.
- The directory lists the **latest** published release; the detail page shows all of them.

## Check before you submit

```bash
docker run --rm --network none -v "$PWD:/p:ro" \
  --entrypoint /go/src/github.com/compscidr/goblog/goblog compscidr/goblog:v0.2.7 \
  validate-plugin /p/plugin.go
# {"name":"hello","display_name":"Hello","version":"1.0.0"}
```

The registry's CI runs this (plus a timeout and memory/process limits), then compares `name` and `version` with your manifest and tag. Your file is executed by the Go interpreter during the check, which is why it runs with networking off.

## Submit

1. Fork this repository and add a line to `registry.yaml`:
   ```yaml
   plugins:
     - repo: goblogplatform/goblog-plugin-hello
     - repo: you/goblog-plugin-yours
   ```
2. Open a pull request. The `validate` workflow must pass.
3. After merge, `https://goblogplatform.github.io/plugins/index.json` and goblog.live/plugins pick it up within a few minutes. New releases of your plugin are picked up automatically on the next scheduled build.

Plugins run inside the goblog process of whoever installs them. Keep them small and readable; the registry is curated and maintainers may decline or remove entries.
