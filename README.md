# goblog plugin registry

The curated list of [goblog](https://github.com/goblogplatform/goblog) plugins behind [goblog.live/plugins](https://goblog.live/plugins).

- `registry.yaml` — the list. Submit your repository via the [issue form](.github/ISSUE_TEMPLATE/submit-plugin.yml) (a pull request by hand is the alternative); see [docs/CONTRACT.md](docs/CONTRACT.md).
- `https://goblogplatform.github.io/plugins/index.json` — the machine-readable index: the latest release of each plugin, with `download_url` (the release asset named by the manifest's `entry`, `plugin.wasm` by default) and its `sha256`, `runtime` and `install_type` (both `wasm`), `allowed_hosts` (the network the module may reach; `[]` for none) and `stars` (GitHub stargazers, the directory's default ordering); `plugins/<name>.json` adds the rendered README, changelog and release history.
- `cmd/registry` — the tool CI runs: `validate` on pull requests, `build` on merge and every six hours.

```bash
go run ./cmd/registry validate                      # every entry
go run ./cmd/registry validate --repo you/plugin    # one entry
go run ./cmd/registry build --out dist              # what gets published
```

Set `GITHUB_TOKEN` to avoid API rate limits. `validate`/`build` download each plugin's module — the release asset named by `entry` in its manifest, `plugin.wasm` by default — and run `goblog validate-plugin` on it in the `compscidr/goblog` Docker image (`--image` to override; Renovate keeps the default current). With snap-installed Docker, set `TMPDIR` to a directory under your home; snap's Docker cannot bind-mount `/tmp`. Resource limits (`--memory`, `--pids-limit`) need cgroup controllers; on rootless Docker they may be downgraded or rejected — pass `--image` to a local build or run on a rootful daemon.

## Submissions

The `Submission` workflow (`.github/workflows/submit.yml`) turns a [submission issue](.github/ISSUE_TEMPLATE/submit-plugin.yml) into a `registry.yaml` pull request without anyone touching Git. Its `check` job (`contents: read` only) parses the `owner/name` from the issue form and runs `registry build` against it in the sandbox; its `respond` job opens the pull request when that passes, or comments on the issue with the failure (or an "already listed" notice) when it doesn't.

**`SUBMIT_TOKEN` is required**, not optional: this organization disables "Allow GitHub Actions to create and approve pull requests", so the default `GITHUB_TOKEN` cannot open pull requests here at all (separately, GitHub also prevents `GITHUB_TOKEN`-created PRs from triggering other workflows, so even where that org setting is allowed, `Validate` wouldn't run on them). Without `SUBMIT_TOKEN` the workflow still validates submissions and comments the result, but a maintainer must add passing entries to `registry.yaml` by hand. Set it as a repository secret: a fine-grained PAT with Contents, Pull requests and Issues write on this repo, minted from a **dedicated machine user or GitHub App** — not a maintainer's personal account, since the bot's comments and commits are attributed to whatever identity the token belongs to.

License: Apache-2.0.
