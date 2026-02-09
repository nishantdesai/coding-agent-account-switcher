class Ags < Formula
  desc "CLI for saving and switching codex and pi auth profiles"
  homepage "https://github.com/nishantdesai/coding-agent-account-switcher"
  license "MIT"

  head "https://github.com/nishantdesai/coding-agent-account-switcher.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/ags"
  end

  test do
    assert_match "ags version", shell_output("#{bin}/ags version")
  end
end
