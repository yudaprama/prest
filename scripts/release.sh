#!/usr/bin/env bash
# scripts/release.sh — one-shot pREST release.
#
# Usage:
#   ./scripts/release.sh                # auto-bump patch (2.0.1 → 2.0.2)
#   ./scripts/release.sh 2.1.0          # explicit version
#   ./scripts/release.sh --dry-run      # show what would happen, change nothing
#   ./scripts/release.sh --no-push      # commit + tag locally, don't push
#
# What it does:
#   1. Sanity check (clean tree, on main, helpers.PrestVersionNumber parses)
#   2. Update helpers/prest.go with the new version
#   3. go test + go build (rollback on failure)
#   4. git commit + tag
#   5. git push (unless --no-push) → triggers CI → GitHub Release + Docker
#
# Safety: if any step fails, the script reverts helpers/prest.go so the
# working tree is left clean. No half-done state.
set -euo pipefail

DRY_RUN=0
NO_PUSH=0
VERSION=""

for arg in "$@"; do
  case "$arg" in
    --dry-run)  DRY_RUN=1 ;;
    --no-push)  NO_PUSH=1 ;;
    -h|--help)
      sed -n '2,17p' "$0"
      exit 0
      ;;
    [0-9]*.[0-9]*.[0-9]*)
      VERSION="$arg"
      ;;
    *)
      echo "unknown arg: $arg" >&2; exit 1 ;;
  esac
done

cd "$(git rev-parse --show-toplevel)"

# trap: if anything fails AFTER we modify helpers/prest.go, revert it.
# This prevents the "file modified but not committed" bug.
CLEANUP_NEEDED=0
cleanup() {
  if [[ "$CLEANUP_NEEDED" -eq 1 ]]; then
    echo "→ reverting helpers/prest.go (release failed mid-way)" >&2
    git checkout helpers/prest.go 2>/dev/null || true
  fi
}
trap cleanup EXIT

# --- sanity ----------------------------------------------------------------
if [[ -n "$(git status --porcelain)" ]]; then
  echo "✗ working tree not clean. Commit or stash first." >&2
  git status --short >&2
  exit 1
fi

BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "$BRANCH" != "main" ]]; then
  echo "✗ not on main (currently on $BRANCH)." >&2
  exit 1
fi

# --- derive version --------------------------------------------------------
CURRENT=$(grep -oE '"[0-9]+\.[0-9]+\.[0-9]+(-[a-z0-9.-]+)?"' helpers/prest.go | head -1 | tr -d '"')
if [[ -z "$CURRENT" ]]; then
  echo "✗ could not parse helpers.PrestVersionNumber" >&2
  exit 1
fi

if [[ -z "$VERSION" ]]; then
  IFS='.' read -r MAJOR MINOR PATCH <<< "${CURRENT%%-*}"
  PATCH=$((PATCH + 1))
  VERSION="$MAJOR.$MINOR.$PATCH"
fi

echo "→ release: $CURRENT → $VERSION"

# --- dry-run ---------------------------------------------------------------
if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "(dry-run: would update helpers/prest.go, test, build, commit, tag, push)"
  exit 0
fi

# --- apply + test + build --------------------------------------------------
echo "→ update helpers/prest.go"
sed -i.bak "s/PrestVersionNumber = \"$CURRENT\"/PrestVersionNumber = \"$VERSION\"/" helpers/prest.go
rm -f helpers/prest.go.bak
CLEANUP_NEEDED=1   # from here on, failure → revert

echo "→ go test"
go test -count=1 -run "TestParseDBConfig|Test_pgURLEnvKey|TestLoad|TestDatabaseURL|TestHTTPPort|Test_Auth|Test_getPrestConfFile|Test_portFromEnv|TestValidateJWTConfig|Test_fetchJWKS|TestExposeDataConfig|Test_parseDatabaseURL" ./config/
go test -count=1 -run TestExtractContextValues ./controllers/
go test -count=1 ./template/

echo "→ go build"
go build -o /tmp/prestd-release-smoke ./cmd/prestd/
BUILT_VERSION=$(/tmp/prestd-release-smoke version 2>/dev/null | awk '{print $NF}')
rm -f /tmp/prestd-release-smoke
if [[ "$BUILT_VERSION" != "$VERSION" ]]; then
  echo "✗ built binary reports '$BUILT_VERSION', expected '$VERSION'" >&2
  exit 1
fi

# --- commit + tag ----------------------------------------------------------
echo "→ git commit + tag v$VERSION"
git add helpers/prest.go
git commit -m "chore: bump to $VERSION"
git tag -f "v$VERSION"
CLEANUP_NEEDED=0   # committed — no more cleanup

# --- push ------------------------------------------------------------------
if [[ "$NO_PUSH" -eq 1 ]]; then
  echo "→ --no-push: done locally (not pushed)"
  exit 0
fi

echo "→ git push origin main v$VERSION"
git push origin main
git push origin "v$VERSION" --force

REPO=$(git remote get-url origin | sed 's|.*github.com/||;s|\.git$||')
echo ""
echo "✓ released v$VERSION"
echo "  CI:        https://github.com/$REPO/actions"
echo "  Releases:  https://github.com/$REPO/releases/tag/v$VERSION"
echo ""
echo "  Watch CI finish:"
echo "    gh run watch --repo $REPO --exit-status"
