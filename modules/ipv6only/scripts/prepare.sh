#!/bin/bash

set +x

arch=$(uname -m)
if [ "$arch" = "aarch64" ]; then
    KUBE_ARCH="arm64"
else
    KUBE_ARCH="amd64"
fi
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/$KUBE_ARCH/kubectl" && \
chmod +x ./kubectl && \
mv ./kubectl /usr/local/bin
