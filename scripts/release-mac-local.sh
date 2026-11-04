#!/usr/bin/env bash
# Builds the two darwin archives of a release on this Mac (when the macOS runners of the release workflow do
# not start), uploads them to the GitHub release and recomputes checksums.txt over every archive in it.
# Usage: scripts/release-mac-local.sh v0.5.0
# Needs: macOS with Xcode command line tools (cgo, tree-sitter), Go, gh (logged in). The release is created
# (from the tag, with generated notes) when it does not exist yet.
set -euo pipefail

tag="${1:?usage: release-mac-local.sh <tag>}"
repo="aidarbn/archmap"
root="$(git rev-parse --show-toplevel)"
work="$(mktemp -d)"
trap 'git -C "$root" worktree remove --force "$work/src" 2>/dev/null || true; rm -rf "$work"' EXIT

git -C "$root" rev-parse -q --verify "refs/tags/${tag}" >/dev/null || { echo "no local tag ${tag} (git fetch --tags)" >&2; exit 1; }
git -C "$root" worktree add --detach "$work/src" "$tag" >/dev/null
mkdir -p "$work/out"

build() { # goarch, cc
  local arch="$1" cc="$2" name="archmap_${tag}_darwin_$1"
  mkdir -p "$work/$name"
  (cd "$work/src" && CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" CC="$cc" \
    go build -trimpath -ldflags "-s -w -X main.version=${tag}" -o "$work/$name/archmap" ./cmd/archmap)
  cp "$work/src/README.md" "$work/src/LICENSE" "$work/$name/"
  file "$work/$name/archmap"
  tar -C "$work" -czf "$work/out/$name.tar.gz" "$name"
}
build arm64 "clang -arch arm64"
build amd64 "clang -arch x86_64"

got="$("$work/archmap_${tag}_darwin_arm64/archmap" version)"
echo "arm64: ${got}"
case "$got" in *"${tag}"*) ;; *) echo "version mismatch: ${got}" >&2; exit 1 ;; esac
if arch -x86_64 true 2>/dev/null; then echo "amd64: $(arch -x86_64 "$work/archmap_${tag}_darwin_amd64/archmap" version)"; fi

if gh release view "$tag" -R "$repo" >/dev/null 2>&1; then
  gh release upload "$tag" -R "$repo" --clobber "$work"/out/*.tar.gz
else
  gh release create "$tag" -R "$repo" --verify-tag --generate-notes "$work"/out/*.tar.gz
fi

# checksums over every archive of the release (the ones from the workflow and the two local ones)
mkdir -p "$work/all"
gh release download "$tag" -R "$repo" -p 'archmap_*' -D "$work/all" --clobber
(cd "$work/all" && shasum -a 256 archmap_* > checksums.txt && cat checksums.txt)
gh release upload "$tag" -R "$repo" --clobber "$work/all/checksums.txt"
