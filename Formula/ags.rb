class Ags < Formula
  desc "CLI for saving and switching codex and pi auth profiles"
  homepage "https://github.com/nishantdesai/coding-agent-account-switcher"
  version "0.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/nishantdesai/coding-agent-account-switcher/releases/download/v#{version}/ags_#{version}_darwin_arm64.tar.gz"
      sha256 "8fcd78178596ceea1f0483019415ac6b4dcdf152d846baeecae06b65880b73e6"
    else
      url "https://github.com/nishantdesai/coding-agent-account-switcher/releases/download/v#{version}/ags_#{version}_darwin_x86_64.tar.gz"
      sha256 "269933a28093855f2927cef18cfdde88443b387238258a32520cb7d479c279a3"
    end
  end

  def install
    bin.install "ags"
  end

  test do
    assert_match "ags version", shell_output("#{bin}/ags version")
  end
end
