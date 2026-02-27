class GogLite < Formula
  desc "CLI for AI agents to operate Gmail / Google Calendar / Google Docs"
  homepage "https://github.com/kubot64/gog-lite"
  version "2026.0227.0332"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kubot64/gog-lite/releases/download/v2026.0227.0332/gog-lite_2026.0227.0332_darwin_arm64.tar.gz"
      sha256 "d38db087ceba57af7f1f834cf7fedc3d37947dd09f8c58429878ee670ca484e1"
    else
      url "https://github.com/kubot64/gog-lite/releases/download/v2026.0227.0332/gog-lite_2026.0227.0332_darwin_amd64.tar.gz"
      sha256 "e9c74a5fa639c34a3e83f2dc9583f8e9c7dba887e111cb086daa6b0d7c59584a"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kubot64/gog-lite/releases/download/v2026.0227.0332/gog-lite_2026.0227.0332_linux_arm64.tar.gz"
      sha256 "2497247eda53a1547cb496ead97069552e360a01de9f7ef02c4b3f4a3a1e4cda"
    else
      url "https://github.com/kubot64/gog-lite/releases/download/v2026.0227.0332/gog-lite_2026.0227.0332_linux_amd64.tar.gz"
      sha256 "73572a909e99a3933d707090f6ce1595f6a8cd1afafe19016c4e43cab0a7e66b"
    end
  end

  def install
    bin.install "gog-lite"
  end

  test do
    system "#{bin}/gog-lite", "--version"
  end
end
