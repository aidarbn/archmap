#!/usr/bin/env bash
# Rewrites Formula/archmap.rb in a homebrew-tap checkout from a release's checksums.txt.
# Usage: scripts/update-tap.sh v0.1.0 [path/to/homebrew-tap]
set -euo pipefail

tag="${1:?usage: update-tap.sh <tag> [tap-dir]}"
tap="${2:-../homebrew-tap}"
repo="aidarbn/archmap"
ver="${tag#v}"
base="https://github.com/${repo}/releases/download/${tag}"

sums="$(curl -fsSL "${base}/checksums.txt")"
sha() {
  local s
  s="$(awk -v f="archmap_${tag}_$1.tar.gz" '$2 == f {print $1}' <<<"$sums")"
  [ -n "$s" ] || { echo "no checksum for $1" >&2; exit 1; }
  echo "$s"
}

mkdir -p "${tap}/Formula"
cat > "${tap}/Formula/archmap.rb" <<RUBY
class Archmap < Formula
  desc "Interactive function-level architecture map of a repository"
  homepage "https://github.com/${repo}"
  version "${ver}"
  license "MIT"

  on_macos do
    on_arm do
      url "${base}/archmap_${tag}_darwin_arm64.tar.gz"
      sha256 "$(sha darwin_arm64)"
    end
    on_intel do
      url "${base}/archmap_${tag}_darwin_amd64.tar.gz"
      sha256 "$(sha darwin_amd64)"
    end
  end

  on_linux do
    on_arm do
      url "${base}/archmap_${tag}_linux_arm64.tar.gz"
      sha256 "$(sha linux_arm64)"
    end
    on_intel do
      url "${base}/archmap_${tag}_linux_amd64.tar.gz"
      sha256 "$(sha linux_amd64)"
    end
  end

  def install
    bin.install "archmap"
  end

  test do
    system "#{bin}/archmap", "version"
  end
end
RUBY
echo "updated ${tap}/Formula/archmap.rb to ${tag}"
