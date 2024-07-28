#!/bin/bash

## Uncomment the following lines to enable debug mode
#set -x
# PS4='+(${LINENO}): '

set -x
echo "$@"

arch=$(uname -m)

install_docker() {
    install_cmd="curl -fsSL https://get.docker.com | sh"
    if ! eval "$install_cmd"; then
        echo "Failed to install docker on bastion node"
    fi
}

install_kubectl() {
    if [ $arch = "aarch64" ]; then
        KUBE_ARCH="arm64"
    else
        KUBE_ARCH="amd64"
    fi
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/$KUBE_ARCH/kubectl" && \
    chmod +x ./kubectl && \
    mv ./kubectl /usr/local/bin
}

main() {
    install_docker
    sleep 5
    install_kubectl
    sleep 5
}
main "$@"
