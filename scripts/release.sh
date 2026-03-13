#!/usr/bin/env bash
# Usage: ./scripts/release.sh <major|minor|patch>
set -euo pipefail

bump="${1:-}"
if [[ "$bump" != "major" && "$bump" != "minor" && "$bump" != "patch" ]]; then
  echo "Usage: $0 <major|minor|patch>" >&2
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

echo "Released ${tag} — goreleaser will build binaries."
