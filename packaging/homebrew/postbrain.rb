class Postbrain < Formula
  desc "Persistent memory and knowledge server for coding agents"
  homepage "https://github.com/simplyblock/postbrain"
  version "0.0.0"
  url "https://github.com/simplyblock/postbrain/releases/download/v0.0.0/postbrain_darwin_arm64.tar.gz"
  sha256 "REPLACE_WITH_RELEASE_SHA256"
  license "Apache-2.0"

  def install
    bin.install "postbrain"
    bin.install "postbrain-cli"
    etc.install "config.example.yaml" => "postbrain.yaml"
  end

  test do
    system "#{bin}/postbrain-cli", "--help"
  end
end
