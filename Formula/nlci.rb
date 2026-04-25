# typed: false
# frozen_string_literal: true

# To release a new version:
# 1. Tag the release: git tag v0.x.0 && git push origin v0.x.0
# 2. Build the release tarball: make release
# 3. Update the url and sha256 below
# 4. Submit a PR to homebrew-core or your tap

class Nlci < Formula
  desc "Natural language interface layer for any CLI tool — on-device AI, no cloud"
  homepage "https://github.com/jakejimenez/nlci"
  # TODO: update url and sha256 on first release
  url "https://github.com/jakejimenez/nlci/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "REPLACE_WITH_SHA256_OF_RELEASE_TARBALL"
  license "MIT"
  head "https://github.com/jakejimenez/nlci.git", branch: "main"

  depends_on "go" => :build
  depends_on :macos

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/nlci"
  end

  def post_install
    # Create the nlci config directory
    (var/"nlci").mkpath
    ohai "nlci installed."
    ohai "Run 'nlci config' to check backend health."
    ohai ""
    ohai "Apple Intelligence backend (macOS 26 + Apple Silicon):"
    ohai "  Build and install the Swift bridge:"
    ohai "    cd #{HOMEBREW_PREFIX}/opt/nlci"
    ohai "    make build-apple && make install-apple"
    ohai ""
    ohai "Ollama backend (any Mac):"
    ohai "  brew install ollama"
    ohai "  ollama pull llama3.2:3b"
    ohai "  ollama serve"
  end

  test do
    assert_match "nlci wraps any CLI tool", shell_output("#{bin}/nlci --help")
    assert_match "apple", shell_output("#{bin}/nlci config")
  end
end
