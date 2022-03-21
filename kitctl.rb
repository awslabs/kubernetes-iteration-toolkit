# Update the version and SHA256 for the CLI when new version is released
require 'formula'
class Kitctl < Formula
  homepage 'https://github.com/awslabs/kubernetes-iteration-toolkit/substrate'
  version '0.0.10'
  if OS.mac? && Hardware::CPU.is_64_bit?
    url 'https://github.com/awslabs/kubernetes-iteration-toolkit/releases/download/v0.0.10/kitctl_v0.0.10_darwin_amd64.zip'
    sha256 'a0f766afe188859a48bb6512d5eb4014cf78a93f33f454eb1e5758ab11d20633'
  else
    echo "Hardware not supported"
    exit 1
  end
  def install
    bin.install 'kitctl'
  end
end