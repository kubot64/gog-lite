class GogLite < Formula
  desc "CLI for AI agents to operate Gmail / Google Calendar / Google Docs"
  homepage "https://github.com/kubot64/gog-lite"
  version "2026.0227.0349"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/kubot64/gog-lite/releases/download/v2026.0227.0349/gog-lite_2026.0227.0349_darwin_arm64.tar.gz"
      sha256 "67bf12f3a29b929bfcd23333d7fb87fddca7929e9fc4efdcda5a5943644f4f10"
    else
      url "https://github.com/kubot64/gog-lite/releases/download/v2026.0227.0349/gog-lite_2026.0227.0349_darwin_amd64.tar.gz"
      sha256 "eb81f21f46a81df425670aa7ab1dbd3e392fab17782bfb54d0856fe76849ebf2"
    end
  end

  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/kubot64/gog-lite/releases/download/v2026.0227.0349/gog-lite_2026.0227.0349_linux_arm64.tar.gz"
      sha256 "f05a2039c801c984e1b3e20471697cc4dd552176df5e33e94bd7bccfe5eb0cff"
    else
      url "https://github.com/kubot64/gog-lite/releases/download/v2026.0227.0349/gog-lite_2026.0227.0349_linux_amd64.tar.gz"
      sha256 "b0db9aa265a63904a89f21eba471c6df46a711dcec0d8d6a2bcdb26ad6cf0275"
    end
  end

  def install
    bin.install "gog-lite"
  end

  test do
    system "#{bin}/gog-lite", "--version"
  end
end
