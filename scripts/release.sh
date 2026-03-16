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
git push origin main
git push origin "$tag"

echo "Released ${tag}"

if [[ "$no_wait" == true ]]; then
  echo "Skipping build poll (--no-wait). Run 'loom update' later to fetch the binary."
  exit 0
fi

echo "Waiting for goreleaser..."

# Wait for the tag-triggered run to appear
sleep 5

# Find the run ID for this specific tag
run_id=""
for i in $(seq 1 30); do
  run_id=$(gh run list --limit 5 --json databaseId,headBranch,status -q ".[] | select(.headBranch==\"${tag}\") | .databaseId")
  if [[ -n "$run_id" ]]; then break; fi
  sleep 5
done
if [[ -z "$run_id" ]]; then
  echo "Could not find workflow run for ${tag}. Check GitHub Actions." >&2
  exit 1
fi

# Poll that specific run
while true; do
  result=$(gh run view "$run_id" --json status,conclusion -q '.status + " " + .conclusion')
  status="${result%% *}"
  conclusion="${result#* }"
  if [[ "$status" == "completed" ]]; then
    if [[ "$conclusion" == "success" ]]; then
      echo "Build succeeded. Updating local binary..."
      loom update
      break
    else
      echo "Build failed (${conclusion}). Check GitHub Actions." >&2
      exit 1
    fi
  fi
  sleep 10
done
