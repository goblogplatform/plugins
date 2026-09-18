# goblog plugin registry

The curated list of [goblog](https://github.com/goblogplatform/goblog) plugins behind [goblog.live/plugins](https://goblog.live/plugins).

- `registry.yaml` — the list. Add your repository in a PR; see [docs/CONTRACT.md](docs/CONTRACT.md).
- `https://goblogplatform.github.io/plugins/index.json` — the machine-readable index (latest release of each plugin, with `download_url` and `sha256`) and `stars` (GitHub stargazers, the directory's default ordering); `plugins/<name>.json` adds the rendered README, changelog and release history.
- `cmd/registry` — the tool CI runs: `validate` on pull requests, `build` on merge and every six hours.

```bash
go run ./cmd/registry validate                      # every entry
go run ./cmd/registry validate --repo you/plugin    # one entry
go run ./cmd/registry build --out dist              # what gets published
```

Set `GITHUB_TOKEN` to avoid API rate limits. `validate`/`build` run `goblog validate-plugin` in the `compscidr/goblog` Docker image (`--image` to override; Renovate keeps the default current). With snap-installed Docker, set `TMPDIR` to a directory under your home; snap's Docker cannot bind-mount `/tmp`. Resource limits (`--memory`, `--pids-limit`) need cgroup controllers; on rootless Docker they may be downgraded or rejected — pass `--image` to a local build or run on a rootful daemon.

## Submissions

The `Submission` workflow (`.github/workflows/submit.yml`) turns a [submission issue](.github/ISSUE_TEMPLATE/submit-plugin.yml) into a `registry.yaml` pull request without anyone touching Git. Its `check` job (`contents: read` only) parses the `owner/name` from the issue form and runs `registry build` against it in the sandbox; its `respond` job opens the pull request when that passes, or comments on the issue with the failure (or an "already listed" notice) when it doesn't. Set a repository secret `SUBMIT_TOKEN` (fine-grained PAT with Contents, Pull requests and Issues write on this repo) so the bot's pull requests trigger the `Validate` check; with the default `GITHUB_TOKEN` they don't (GitHub prevents token-created PRs from starting workflows).

License: Apache-2.0.
