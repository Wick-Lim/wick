# wlim

Minimal npm-like installer in Go. Resolves semver ranges, fetches tarballs from the npm registry, and installs into `node_modules`.

## Install

- Prereq: Go 1.21+ (tested with 1.23).

Option A — build and copy to PATH:
```
go build -o wlim
install -m 0755 wlim ~/.local/bin/wlim   # or: sudo install -m 0755 wlim /usr/local/bin/wlim
# ensure ~/.local/bin is on PATH
```

Option B — go install from repo root:
```
export GOBIN=${GOBIN:-$HOME/.local/bin}
export PATH=$GOBIN:$PATH
go install
```

## Usage

```
wlim install [<name>[@version|@range] ...]

# examples
wlim install express
wlim install express@latest
wlim install react@^18
wlim install @types/node@~20
wlim install react react-dom @types/react@^18  # multi-root install
wlim install --frozen-lockfile                 # install strictly from wlim.lock

# install into a specific project directory
wlim install express --dir ./my-app

# remove packages from project (store is kept)
wlim remove react react-dom

# update lockfile to latest for roots (and reinstall)
wlim update               # all roots
wlim update react         # only react
wlim update --policy minor  # keep major, update to highest minor/patch
wlim update --policy patch  # keep major.minor, update to highest patch

# clean unreferenced store entries per wlim.lock
wlim clean                # remove unused from store
wlim clean --dry-run      # only show actions

# list lockfile contents
wlim list                 # roots and packages
wlim list --json          # machine-readable JSON
wlim list --format yaml   # YAML output
```

Notes:
- Uses a pnpm-like global store at `~/.wlim/store/v3` (override with `WLIM_STORE_DIR` or `--store-dir`).
- Creates symlinks from `<projectDir>/node_modules/<name>` to the store; each package’s own `node_modules` links its dependencies.
- Adds direct-dependency bins to `<projectDir>/node_modules/.bin`.
- Parallel installs: use `--concurrency N` to control worker count.
- Writes `wlim.lock` capturing the resolved graph.
- Installs can also consume an existing `wlim.lock` (exact versions pinned).
- Basic semver ranges are supported via Masterminds/semver.
- Integrity verification via `dist.integrity` (SRI) or `shasum` when available.
- Hoisting/deduplication are not implemented yet.

Config:
- Optional `wlim.json` in project root supports:
  - `registry`: default registry base URL
  - `storeDir`: override store path
  - `concurrency`: default parallelism for install/update
  - Precedence: flags > env > `wlim.json` > defaults

Cache:
- Registry metadata cached at `~/.wlim/cache` (override with `WLIM_CACHE_DIR`).
- TTL via `WLIM_CACHE_TTL_SECONDS` (0 disables cache, -1 unlimited).

Logging:
- Use `-v/--verbose` to print download and progress details.

Registry:
- Override the npm registry
  - Flag: `--registry https://registry.npmjs.org`
  - Env: `WLIM_REGISTRY=https://your-registry.example.com`
  - Precedence: flag overrides env.

Remove:
- `wlim remove <pkg> [...]` removes project links and updates the lockfile; add `--clean-store` to also prune the global store.

List:
- `wlim list` prints roots and packages; `--json` outputs a machine-readable format.
Version
- `wlim version` prints the version (set via Go ldflags). `make build` injects the current tag.


Release via Homebrew Tap (optional)
- Configure `.goreleaser.yaml` (already included). Create `wicklim/homebrew-tap` repo.
- Tag and release:
  - `git tag v0.1.0 && git push --tags`
  - `goreleaser release --clean`
- Install:
  - `brew tap wicklim/tap && brew install wlim`
