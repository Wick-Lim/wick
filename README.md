# wick

Minimal npm-like installer in Go. Resolves semver ranges, fetches tarballs from the npm registry, and installs into `node_modules`.

## Install

- Prereq: Go 1.21+ (tested with 1.23).

Option A — build and copy to PATH:
```
go build -o wick
install -m 0755 wick ~/.local/bin/wick   # or: sudo install -m 0755 wick /usr/local/bin/wick
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
wick install [<name>[@version|@range] ...]

# examples
wick install express
wick install express@latest
wick install react@^18
wick install @types/node@~20
wick install react react-dom @types/react@^18  # multi-root install
wick install --frozen-lockfile                 # install strictly from wick.lock

# install into a specific project directory
wick install express --dir ./my-app

# remove packages from project (store is kept)
wick remove react react-dom

# update lockfile to latest for roots (and reinstall)
wick update               # all roots
wick update react         # only react
wick update --policy minor  # keep major, update to highest minor/patch
wick update --policy patch  # keep major.minor, update to highest patch

# clean unreferenced store entries per wick.lock
wick clean                # remove unused from store
wick clean --dry-run      # only show actions

# list lockfile contents
wick list                 # roots and packages
wick list --json          # machine-readable JSON
wick list --format yaml   # YAML output
```

Notes:
- Uses a pnpm-like global store at `~/.wick/store/v3` (override with `WICK_STORE_DIR` or `--store-dir`).
- Creates symlinks from `<projectDir>/node_modules/<name>` to the store; each package’s own `node_modules` links its dependencies.
- Adds direct-dependency bins to `<projectDir>/node_modules/.bin`.
- Parallel installs: use `--concurrency N` to control worker count.
- Writes `wick.lock` capturing the resolved graph.
- Installs can also consume an existing `wick.lock` (exact versions pinned).
- Basic semver ranges are supported via Masterminds/semver.
- Integrity verification via `dist.integrity` (SRI) or `shasum` when available.
- Hoisting/deduplication are not implemented yet.

Config:
- Optional `wick.json` in project root supports:
  - `registry`: default registry base URL
  - `storeDir`: override store path
  - `concurrency`: default parallelism for install/update
  - Precedence: flags > env > `wick.json` > defaults

Cache:
- Registry metadata cached at `~/.wick/cache` (override with `WICK_CACHE_DIR`).
- TTL via `WICK_CACHE_TTL_SECONDS` (0 disables cache, -1 unlimited).

Logging:
- Use `-v/--verbose` to print download and progress details.

Registry:
- Override the npm registry
  - Flag: `--registry https://registry.npmjs.org`
  - Env: `WICK_REGISTRY=https://your-registry.example.com`
  - Precedence: flag overrides env.

Remove:
- `wick remove <pkg> [...]` removes project links and updates the lockfile; add `--clean-store` to also prune the global store.

List:
- `wick list` prints roots and packages; `--json` outputs a machine-readable format.
Version
- `wick version` prints the version (set via Go ldflags). `make build` injects the current tag.


Release via Homebrew Tap (optional)
- Configure `.goreleaser.yaml` (already included). Create `wicklim/homebrew-tap` repo.
- Tag and release:
  - `git tag v0.1.0 && git push --tags`
  - `goreleaser release --clean`
- Install:
  - `brew tap wicklim/tap && brew install wick`
