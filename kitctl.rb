# Update the version and SHA256 for the CLI when new version is released
require 'formula'
class Kitctl < Formula
  homepage 'https://github.com/awslabs/kubernetes-iteration-toolkit/substrate'
  version '0.0.13'
  if OS.mac? && Hardware::CPU.is_64_bit?
    url 'https://github.com/awslabs/kubernetes-iteration-toolkit/releases/download/v0.0.13/kitctl_v0.0.13_darwin_amd64.zip'
    sha256 '83c18bd63f1026171b5fb19f28f3e1896be1b1308a99f86b6f9579cd857169a0'
  else
    echo "Hardware not supported"
    exit 1
  end
  def install
    bin.install 'kitctl'
  end
end