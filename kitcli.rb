# Update the version and SHA256 for the CLI when new version is released
require 'formula'
class Kitcli < Formula
  homepage 'https://github.com/awslabs/kubernetes-iteration-toolkit/substrate'
  version '$VERSION'
  if OS.mac? && Hardware::CPU.is_64_bit?
    url 'https://github.com/awslabs/kubernetes-iteration-toolkit/releases/download/$VERSION/kitcli_$VERSION_darwin_amd64.zip'
    sha256 '$SHA256'
  else
    echo "Hardware not supported"
    exit 1
  end
  def install
    bin.install 'kitcli'
  end
end