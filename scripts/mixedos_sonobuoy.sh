#!/bin/bash

# Usage: sh mixedos_sonobouy.sh install 0.56.16 amd64
# Usage: sh mixedos_sonobouy.sh delete

if [ -z "$2" ]
then
    version="0.56.16"
else
    version=$2
fi

if [ -z "$3" ]
then
    arch="amd64"
else
    arch=$3
fi

installation(){
    echo "Installing sonobuoy version ${version} for mixedos validation"
    git clone https://github.com/phillipsj/my-sonobuoy-plugins.git
    wait
    wget -q https://github.com/vmware-tanzu/sonobuoy/releases/download/v${version}/sonobuoy_${version}_linux_${arch}.tar.gz
    wait
    tar -xvf sonobuoy_${version}_linux_amd64.tar.gz
    chmod +x sonobuoy && mv sonobuoy /usr/local/bin/sonobuoy
}

cleanup(){
    echo "Deleting sonobuoy installer"
    rm -rf my-sonobuoy-plugins
    rm -rf sonobuoy_*
    rm -rf /usr/local/bin/sonobuoy
}

if [ "$1" == "install" ];
then
    installation
elif [ "$1" == "delete" ];
then
    cleanup
else
    echo "Invalid argument, please pass required arg [install or delete]"
fi


