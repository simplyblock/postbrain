class PostbrainClient < Formula
  desc "Postbrain CLI client"
  homepage "https://github.com/simplyblock/postbrain"
  version "0.0.0"
  url "https://github.com/simplyblock/postbrain/releases/download/v0.0.0/postbrain-client_darwin_arm64.tar.gz"
  sha256 "REPLACE_WITH_RELEASE_SHA256"
  license "Apache-2.0"

  def install
    bin.install "postbrain-cli"
  end

  test do
    system "#{bin}/postbrain-cli", "--help"
  end
end
