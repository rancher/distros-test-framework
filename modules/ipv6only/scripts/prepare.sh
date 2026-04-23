#!/bin/bash

# This script prepares bastion node with installing kubectl
# set +x

KUBECTL_VERSION="v1.35.3"
arch=$(uname -m)
if [ "$arch" = "aarch64" ]; then
    KUBE_ARCH="arm64"
    KUBECTL_SHA256="6f0cd088a82dde5d5807122056069e2fac4ed447cc518efc055547ae46525f14"
else
    KUBE_ARCH="amd64"
    KUBECTL_SHA256="fd31c7d7129260e608f6faf92d5984c3267ad0b5ead3bced2fe125686e286ad6"
fi

curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${KUBE_ARCH}/kubectl" && \
echo "${KUBECTL_SHA256}  kubectl" | sha256sum -c - && \
chmod +x ./kubectl && \
mv ./kubectl /usr/local/bin
