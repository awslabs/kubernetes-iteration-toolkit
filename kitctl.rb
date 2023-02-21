# Update the version and SHA256 for the CLI when new version is released
require 'formula'
class Kitctl < Formula
  homepage 'https://github.com/awslabs/kubernetes-iteration-toolkit/substrate'
  version '0.0.22'
  if OS.mac? && Hardware::CPU.is_64_bit?
    url 'https://github.com/awslabs/kubernetes-iteration-toolkit/releases/download/v0.0.22/kitctl_v0.0.22_darwin_amd64.zip'
    sha256 '8e7fd7a6466f97037788b498a2a98d3f5eb980a7db5c3b678c664b66fef6d007'
  else
    echo "Hardware not supported"
    exit 1
  end
  def install
    bin.install 'kitctl'
  end
end