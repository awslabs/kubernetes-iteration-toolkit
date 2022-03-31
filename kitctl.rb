# Update the version and SHA256 for the CLI when new version is released
require 'formula'
class Kitctl < Formula
  homepage 'https://github.com/awslabs/kubernetes-iteration-toolkit/substrate'
  version '0.0.12'
  if OS.mac? && Hardware::CPU.is_64_bit?
    url 'https://github.com/awslabs/kubernetes-iteration-toolkit/releases/download/v0.0.12/kitctl_v0.0.12_darwin_amd64.zip'
    sha256 'a2514c7f3b2e558c97c7b155f683ccd45aa19b68bac7683069f921e1ea813af7'
  else
    echo "Hardware not supported"
    exit 1
  end
  def install
    bin.install 'kitctl'
  end
end