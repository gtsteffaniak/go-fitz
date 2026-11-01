#!/usr/bin/env bash
# Fetch the pinned MuPDF source used as the porting reference.
#
# The source tree is downloaded into a gitignored directory and verified
# against a pinned commit SHA. Re-running is idempotent: an existing checkout
# at the right commit is reused.
#
# Usage: scripts/fetch-mupdf-source.sh [destination]
#   destination defaults to .mupdf-src/ at the repo root.

set -euo pipefail

MUPDF_VERSION="1.28.0"
MUPDF_COMMIT="205b8cf43551279d1215e88fe2845c5d595bade9"
MUPDF_REPO="https://github.com/ArtifexSoftware/mupdf.git"

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
dest="${1:-$repo_root/.mupdf-src}"

if [ -d "$dest/.git" ]; then
  current="$(git -C "$dest" rev-parse HEAD 2>/dev/null || true)"
  if [ "$current" = "$MUPDF_COMMIT" ]; then
    echo "MuPDF $MUPDF_VERSION already present at $dest"
    exit 0
  fi
  echo "error: $dest is at $current, expected $MUPDF_COMMIT" >&2
  echo "remove it and re-run" >&2
  exit 1
fi

mkdir -p "$dest"
git -C "$dest" init -q
git -C "$dest" remote add origin "$MUPDF_REPO"
git -C "$dest" fetch -q --depth 1 origin "$MUPDF_COMMIT"
git -C "$dest" checkout -q FETCH_HEAD

actual="$(git -C "$dest" rev-parse HEAD)"
if [ "$actual" != "$MUPDF_COMMIT" ]; then
  echo "error: fetched $actual, expected $MUPDF_COMMIT" >&2
  exit 1
fi

echo "MuPDF $MUPDF_VERSION fetched at $dest ($MUPDF_COMMIT)"
