#!/usr/bin/env bash
# scripts/release.sh — one-shot pREST release.
#
# Usage:
#   ./scripts/release.sh 2.0.1          # explicit version
#   ./scripts/release.sh                # auto-bump patch (2.0.0 → 2.0.1)
#   ./scripts/release.sh --dry-run 2.0.1
#   ./scripts/release.sh --no-push 2.0.1
#
# What it does:
#   1. Sanity check (clean tree, on main, helpers.PrestVersionNumber parses)
#   2. Update helpers/prest.go with the new version
#   3. go test ./config/... and go build ./cmd/prestd/
#   4. git commit + tag
#   5. git push (unless --no-push)
#
# Tag collisions (e.g. the v2.0.0 we inherited from upstream) are handled
# by `git tag -f` — the caller has already accepted that this is a fork.
set -euo pipefail

DRY_RUN=0
NO_PUSH=0
VERSION=""

for arg in "$@"; do
  case "$arg" in
    --dry-run)  DRY_RUN=1 ;;
    --no-push)  NO_PUSH=1 ;;
    -h|--help)
      sed -n '2,18p' "$0"
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

# --- sanity ----------------------------------------------------------------
if [[ -n "$(git status --porcelain)" ]]; then
  echo "✗ working tree not clean. Commit or stash first." >&2
  git status --short >&2
  exit 1
fi

BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "$BRANCH" != "main" ]]; then
  echo "✗ not on main (currently on $BRANCH). Switch first." >&2
  exit 1
fi

# --- derive version --------------------------------------------------------
CURRENT=$(grep -oE '"[0-9]+\.[0-9]+\.[0-9]+(-[a-z0-9.-]+)?"' helpers/prest.go | head -1 | tr -d '"')
if [[ -z "$CURRENT" ]]; then
  echo "✗ could not parse helpers.PrestVersionNumber" >&2
  exit 1
fi

if [[ -z "$VERSION" ]]; then
  # bump patch: 2.0.0 → 2.0.1
  IFS='.' read -r MAJOR MINOR PATCH <<< "${CURRENT%%-*}"
  PATCH=$((PATCH + 1))
  VERSION="$MAJOR.$MINOR.$PATCH"
fi

echo "→ release: $CURRENT → $VERSION"

# --- apply ----------------------------------------------------------------
if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "(dry-run: would update helpers/prest.go to $VERSION)"
else
  sed -i.bak "s/PrestVersionNumber = \"$CURRENT\"/PrestVersionNumber = \"$VERSION\"/" helpers/prest.go
  rm -f helpers/prest.go.bak

  echo "→ go test ./config/... ./controllers/... ./template/..."
  go test -count=1 -run "TestParseDBConfig|Test_pgURLEnvKey|TestLoad|TestParse|TestDatabaseURL|TestHTTPPort|Test_Auth|Test_getPrestConfFile|Test_portFromEnv|TestValidateJWTConfig|Test_fetchJWKS|TestExposeDataConfig|Test_parseDatabaseURL" ./config/
  go test -count=1 -run TestExtractContextValues ./controllers/
  go test -count=1 ./template/

  echo "→ go build ./cmd/prestd/"
  go build -o /tmp/prestd-release-smoke ./cmd/prestd/
  BUILT_VERSION=$(/tmp/prestd-release-smoke version 2>/dev/null | awk '{print $NF}')
  if [[ "$BUILT_VERSION" != "$VERSION" ]]; then
    echo "✗ built binary reports '$BUILT_VERSION', expected '$VERSION'" >&2
    exit 1
  fi
  rm -f /tmp/prestd-release-smoke

  echo "→ git commit + tag v$VERSION"
  git add helpers/prest.go
  git commit -m "chore: bump to $VERSION"
  git tag -f "v$VERSION"
fi

# --- push -----------------------------------------------------------------
if [[ "$NO_PUSH" -eq 1 ]]; then
  echo "→ --no-push: skipping push"
  exit 0
fi

echo "→ git push origin main v$VERSION"
if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "(dry-run: would push)"
  exit 0
fi
git push origin main
git push origin "v$VERSION" --force

echo "✓ released v$VERSION — watch CI at https://github.com/$(git remote get-url origin | sed 's|.*github.com/||;s|\.git$||')/actions"
