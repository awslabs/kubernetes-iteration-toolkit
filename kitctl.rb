# Update the version and SHA256 for the CLI when new version is released
require 'formula'
class Kitctl < Formula
  homepage 'https://github.com/awslabs/kubernetes-iteration-toolkit/substrate'
  version '0.0.16'
  if OS.mac? && Hardware::CPU.is_64_bit?
    url 'https://github.com/awslabs/kubernetes-iteration-toolkit/releases/download/v0.0.16/kitctl_v0.0.16_darwin_amd64.zip'
    sha256 '490cfae9cd79933b637532e105c46db9f39b53c7a3542b1e4e58872743fec5b3'
  else
    echo "Hardware not supported"
    exit 1
  end
  def install
    bin.install 'kitctl'
  end
end