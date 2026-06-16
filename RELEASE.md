# pREST Release Guide

> **Goal:** From "I have changes to ship" to "binary on GitHub Releases" in under 60 seconds.
> Any agent or human can follow this document end-to-end without asking questions.

---

## TL;DR — The 60-second release

```bash
./scripts/release.sh
```

That's it. The script does everything: bump version, test, build, commit, tag, push. CI takes over from there and publishes binaries + Docker images automatically.

For the next 60 seconds, you can watch CI finish:

```bash
gh run watch --repo yudaprama/prest --exit-status
```

Done. Release is live at <https://github.com/yudaprama/prest/releases>.

---

## Prerequisites (one-time setup, already done)

| Requirement | Status |
|---|---|
| `scripts/release.sh` exists and is executable | ✅ committed |
| `helpers/prest.go` has `PrestVersionNumber` | ✅ |
| `.github/workflows/release.yml` builds on `v*` tags | ✅ |
| `.github/workflows/build.yml` pushes Docker on `v*` tags | ✅ |
| `gh` CLI authenticated | ✅ |
| Git remote is `yudaprama/prest` | ✅ |

No other setup needed. Ever.

---

## The release script

**Location:** `scripts/release.sh`

### Commands

| Command | What it does |
|---|---|
| `./scripts/release.sh` | Auto-bump patch (e.g. `2.0.1` → `2.0.2`), commit, tag, push |
| `./scripts/release.sh 2.1.0` | Use explicit version, commit, tag, push |
| `./scripts/release.sh --dry-run` | Show what would happen, change nothing |
| `./scripts/release.sh --no-push` | Commit + tag locally, don't push |
| `./scripts/release.sh -h` | Show help |

### What the script does (step by step)

1. **Sanity check** — working tree must be clean, must be on `main`
2. **Derive version** — reads `helpers.PrestVersionNumber`, bumps patch if no arg
3. **Update** `helpers/prest.go` with `sed`
4. **Test** — `go test ./config/ ./controllers/ ./template/`
5. **Build** — `go build ./cmd/prestd/`, verify `./prestd version` matches
6. **Commit** — `git commit -m "chore: bump to X.Y.Z"`
7. **Tag** — `git tag vX.Y.Z`
8. **Push** — `git push origin main && git push origin vX.Y.Z`

### What CI does automatically (after push)

Two workflows fire on `v*` tag push:

**`release.yml`** (binaries → GitHub Releases):
- Builds 6 targets: `darwin-amd64`, `darwin-arm64`, `linux-amd64`, `linux-arm64`, `windows-amd64`, `windows-arm64`
- Packages as `.tar.gz` (Unix) or `.zip` (Windows) + `.sha256` checksum
- Creates GitHub Release via `softprops/action-gh-release@v2`
- Release is public immediately

**`build.yml`** (Docker images):
- Runs `goreleaser release --clean`
- Pushes to Docker Hub (`prest/prest:X.Y.Z`) and GHCR (`ghcr.io/prest/prest:X.Y.Z`)
- Also tags `:latest`
- **Note:** requires `DOCKER_LOGIN` + `DOCKER_PASSWORD` secrets for Docker Hub. GHCR uses built-in `GITHUB_TOKEN`. If Docker Hub secrets are missing, this step fails but **binaries are already published** — Docker is not blocking.

---

## Manual release (if script fails or you want control)

```bash
# 1. Bump version
sed -i '' 's/PrestVersionNumber = "2.0.1"/PrestVersionNumber = "2.0.2"/' helpers/prest.go

# 2. Test + build
go test ./config/... && go build ./cmd/prestd/

# 3. Commit + tag + push
git add helpers/prest.go
git commit -m "chore: bump to 2.0.2"
git tag v2.0.2
git push origin main
git push origin v2.0.2
```

CI does the rest. Check status:

```bash
gh run list --repo yudaprama/prest --limit 3
gh release view v2.0.2 --repo yudaprama/prest
```

---

## Version numbering

| Type | When to use | Example |
|---|---|---|
| **Patch** (`2.0.2`) | Bug fixes, config changes, no new API | Default — use `./scripts/release.sh` |
| **Minor** (`2.1.0`) | New features, new endpoints, backward-compatible | `./scripts/release.sh 2.1.0` |
| **Major** (`3.0.0`) | Breaking changes, API removal | `./scripts/release.sh 3.0.0` |

Rules:
- Version lives in **one place**: `helpers/prest.go::PrestVersionNumber`
- Tag format: `vX.Y.Z` (with `v` prefix)
- Tags are immutable once published — **never force-overwrite a tag that has a release**. Always create a new version instead.
- Pre-release: append `-rc.N` (e.g. `v2.1.0-rc1`). CI marks it as prerelease automatically.

---

## Verify a release

After CI completes (usually 1-2 minutes):

```bash
# Check release exists
gh release view v2.0.2 --repo yudaprama/prest

# Download and verify binary
gh release download v2.0.2 --repo yudaprama/prest -p "*-darwin-arm64.tar.gz"
tar xzf prest-v2.0.2-darwin-arm64.tar.gz
./prest version   # should print ... 2.0.2
```

---

## Troubleshooting

### "working tree not clean"
Commit or stash your changes first. The script refuses to run on a dirty tree.

### "not on main"
```bash
git checkout main && git pull
```

### CI `release.yml` failed
```bash
gh run view --repo yudaprama/prest --log-failed
```
Common cause: tag wasn't pushed (`git push origin vX.Y.Z`).

### CI `build.yml` (Docker) failed with "Password required"
Docker Hub secrets not configured. Binaries are still published (release.yml is independent). To fix Docker:
1. Go to <https://github.com/yudaprama/prest/settings/secrets/actions>
2. Add `DOCKER_LOGIN` and `DOCKER_PASSWORD` with your Docker Hub credentials
3. Re-run: `gh run rerun <run-id> --repo yudaprama/prest`

### Tests fail in CI but pass locally
The `test.yml` workflow runs the full test suite including tests that need a live PostgreSQL. These are pre-existing failures unrelated to config changes. Check `release.yml` status instead — that's the one that produces binaries.

### Tag already exists
```bash
git tag -d vX.Y.Z                    # delete local
git push origin :refs/tags/vX.Y.Z    # delete remote
# now re-tag
```
**Only do this if no GitHub Release exists for that tag.** If a release exists, bump to the next version instead.

---

## CI workflow reference

| Workflow | File | Triggers on | Produces |
|---|---|---|---|
| Binaries + Release | `.github/workflows/release.yml` | `v*` tag push | GitHub Release with 6-platform binaries |
| Docker images | `.github/workflows/build.yml` | `v*` tag push + `main` push | `prest/prest:X.Y.Z` on Docker Hub + GHCR |
| Tests | `.github/workflows/test.yml` | `v*` tag push + `main` push | Pass/fail status (non-blocking for release) |
| Lint | `.github/workflows/lint.yml` | `main` push + PRs | Pass/fail status |
| CodeQL | `.github/workflows/codeql-analysis.yml` | `main` push + PRs | Security analysis |
| Misspell | `.github/workflows/misspell.yml` | `main` push + PRs | Spelling check |

---

## The `.goreleaser.yml` file

GoReleaser config is at repo root. It controls:
- Binary name: `prestd` (from `cmd/prestd/main.go`)
- Cross-compile targets: 30+ OS/arch combos
- Docker images: `prest/prest` + `ghcr.io/prest/prest`
- Changelog: auto-generated from commits, excludes `test:`, typos, merge commits

To validate changes to `.goreleaser.yml`:
```bash
goreleaser check
```

---

## File map

| File | Purpose |
|---|---|
| `helpers/prest.go` | Version constant (`PrestVersionNumber`) |
| `scripts/release.sh` | One-shot release script |
| `.goreleaser.yml` | GoReleaser config (binary + Docker) |
| `.github/workflows/release.yml` | CI: build binaries → GitHub Releases |
| `.github/workflows/build.yml` | CI: GoReleaser → Docker Hub + GHCR |
| `Dockerfile` | Plugin-enabled Docker image |
| `Dockerfile.noplugins` | Slim Docker image (no plugins) |

---

*Last updated: 2026-06-16. If anything in this doc is wrong, the script is the source of truth.*
