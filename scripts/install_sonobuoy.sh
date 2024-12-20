#!/bin/bash

# Usage: sh mixedos_sonobuoy.sh runs with default values
# Usage: sh mixedos_sonobouy.sh install 0.56.17 arm64
# Usage: sh mixedos_sonobouy.sh delete

action=${1:-install}
version=${2:-0.57.2}
arch=${3:-amd64}

# adopt golang error handling in bash check variables are passed in appropriately - if not return appropriate error message

installation(){
    echo "Installing sonobuoy version ${version} for mixedos validation"
    if [ ! -d "my-sonobuoy-plugins" ]; 
    then
        echo "Cloning repo: https://github.com/phillipsj/my-sonobuoy-plugins.git"
        git clone https://github.com/phillipsj/my-sonobuoy-plugins.git
    fi
    wait
    echo "Downloading sonobouy installer..."
    if [[ $(command -v wget) ]]; then
        wget -q https://github.com/vmware-tanzu/sonobuoy/releases/download/v"${version}"/sonobuoy_"${version}"_linux_"${arch}".tar.gz -O sonobuoy.tar.gz
    elif [[ $(command -v curl) ]]; then
        curl -s https://github.com/vmware-tanzu/sonobuoy/releases/download/v"${version}"/sonobuoy_"${version}"_linux_"${arch}".tar.gz --output sonobuoy.tar.gz
        wait
        sleep 10
    else
        echo "Unable to use wget or curl to download sonobuoy installer, consider a networking error or an under configured OS if this error persists"
        wait
        sleep 10
    fi
    wait
    tar -xvf sonobuoy.tar.gz
    wait
    mv sonobuoy /usr/local/bin/sonobuoy
    chmod +x /usr/local/bin/sonobuoy
}

deletion(){
    #has_bin sonobuoy
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
