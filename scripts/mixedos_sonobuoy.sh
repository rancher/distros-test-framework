#!/bin/bash

# Usage: sh mixedos_sonobouy.sh install 0.56.17 amd64
# Usage: sh mixedos_sonobouy.sh delete

action=$1
version=$2
arch=$3

if [ -z "$version" ] || [[ -n "$version" && "$version" != *.* ]];
then
    version="0.56.17"
fi

if [ -z "$arch" ] || [[ "$arch" != "arm" ]];
then
    arch="amd64"
fi

installation(){
    echo "Installing sonobuoy version ${version} for mixedos validation"
    if [ ! -d "my-sonobuoy-plugins" ]; 
    then
        echo "Cloning repo: https://github.com/phillipsj/my-sonobuoy-plugins.git"
        git clone https://github.com/phillipsj/my-sonobuoy-plugins.git
    fi
    wait
    if [ ! -f "sonobuoy_${version}_linux_${arch}.tar.gz" ];
    then
        echo "Downloading sonobouy installer"
        wget -q https://github.com/vmware-tanzu/sonobuoy/releases/download/v${version}/sonobuoy_${version}_linux_${arch}.tar.gz
    fi
    wait
    tar -xvf sonobuoy_${version}_linux_${arch}.tar.gz
    chmod +x sonobuoy && mv sonobuoy /usr/local/bin/sonobuoy
}

deletion(){
    echo "Deleting sonobuoy installer"
    rm -rf my-sonobuoy-plugins
    rm -rf sonobuoy_*
    rm -rf /usr/local/bin/sonobuoy
}

if [ "$action" == "install" ];
then
    installation
elif [ "$action" == "delete" ];
then
    deletion
else
    echo "Invalid argument, please pass required arg [install or delete]"
fi


