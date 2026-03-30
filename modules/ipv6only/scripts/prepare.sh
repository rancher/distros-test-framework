#!/bin/bash

# This script prepares bastion node with installing kubectl
# set +x

arch=$(uname -m)
if [ "$arch" = "aarch64" ]; then
    KUBE_ARCH="arm64"
else
    KUBE_ARCH="amd64"
fi

KUBECTL_VERSION=$(curl -L -s https://dl.k8s.io/release/stable.txt)
KUBECTL_SHA256=$(curl -sL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${KUBE_ARCH}/kubectl.sha256")
curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${KUBE_ARCH}/kubectl" && \
echo "${KUBECTL_SHA256}  kubectl" | sha256sum -c - && \
chmod +x ./kubectl && \
mv ./kubectl /usr/local/bin
