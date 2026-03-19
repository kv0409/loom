#!/usr/bin/env bash
# Usage: ./scripts/release.sh [--no-wait] <major|minor|patch>
set -euo pipefail

no_wait=false
while [[ "${1:-}" == --* ]]; do
  case "$1" in
    --no-wait) no_wait=true; shift ;;
    *) echo "Unknown flag: $1" >&2; exit 1 ;;
  esac
done

bump="${1:-}"
if [[ "$bump" != "major" && "$bump" != "minor" && "$bump" != "patch" ]]; then
  echo "Usage: $0 [--no-wait] <major|minor|patch>" >&2
  exit 1
fi

cd "$(git rev-parse --show-toplevel)"

cur=$(grep '^VERSION=' Makefile | cut -d= -f2)
IFS='.' read -r major minor patch_v <<< "$cur"

case "$bump" in
  major) major=$((major + 1)); minor=0; patch_v=0 ;;
  minor) minor=$((minor + 1)); patch_v=0 ;;
  patch) patch_v=$((patch_v + 1)) ;;
esac

next="${major}.${minor}.${patch_v}"
tag="v${next}"

echo "Bumping ${cur} → ${next}"

sed -i '' "s/^VERSION=.*/VERSION=${next}/" Makefile
git add Makefile
git commit -m "chore: bump version to ${next}"
git tag -a "$tag" -m "Release ${tag}"

# Push commit and tag in a single round-trip
git push origin main "$tag"

# Build locally immediately — no need to wait for goreleaser
echo "Building locally..."
make install
echo "Installed ${tag} locally."

# Restart daemon if running
if command -v loom &>/dev/null; then
  loom restart 2>/dev/null && echo "Daemon restarted." || true
fi

echo "Released ${tag}"

if [[ "$no_wait" == true ]]; then
  echo "Goreleaser building in background. Run 'gh run watch' to monitor."
  exit 0
fi

echo "Waiting for goreleaser (for GitHub Releases)..."

# Poll for the workflow run — no artificial sleep
run_id=""
for i in $(seq 1 30); do
  run_id=$(gh run list --limit 5 --json databaseId,headBranch,status -q ".[] | select(.headBranch==\"${tag}\") | .databaseId")
  if [[ -n "$run_id" ]]; then break; fi
  sleep 3
done
if [[ -z "$run_id" ]]; then
  echo "Could not find workflow run for ${tag}. Check GitHub Actions." >&2
  exit 1
fi

# Poll every 5s instead of 10s
while true; do
  result=$(gh run view "$run_id" --json status,conclusion -q '.status + " " + .conclusion')
  status="${result%% *}"
  conclusion="${result#* }"
  if [[ "$status" == "completed" ]]; then
    if [[ "$conclusion" == "success" ]]; then
      echo "Goreleaser succeeded. GitHub Release binaries are live."
      break
    else
      echo "Goreleaser failed (${conclusion}). Check GitHub Actions." >&2
      exit 1
    fi
  fi
  sleep 5
done
