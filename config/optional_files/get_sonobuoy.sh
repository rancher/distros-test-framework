#! /bin/bash
get_sono() {
    has_bin wget
    _arch
    arch=$(if [ "$(uname -m)" = "x86_64" ]; then echo "amd64"; else echo "arm64"; fi)
    wget https://github.com/vmware-tanzu/sonobuoy/releases/download/v0.57.1/sonobuoy_0.57.1_linux_"${arch}".tar.gz
    sudo tar -xzf sonobuoy_0.57.1_linux_"${arch}".tar.gz -C /usr/local/bin
}
