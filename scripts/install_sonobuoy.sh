#!/bin/bash

# Usage: sh install_sonobuoy.sh runs with default values
# Usage: sh install_sonobouy.sh install 0.56.17 arm64
# Usage: sh install_sonobouy.sh delete

action=${1:-install}
version=${2:-0.57.2}
arch=${3:-amd64}
mixed_plugins_url="git clone https://github.com/phillipsj/my-sonobuoy-plugins.git"
sonobuoy_url="https://github.com/vmware-tanzu/sonobuoy/releases/download/v${version}/sonobuoy_${version}_linux_${arch}.tar.gz"
max_retries=5
retry_delay=11
# adopt golang error handling in bash check variables are passed in appropriately - if not return appropriate error message

download_retry(){
  i=1
 
  until $1 || [ $i -gt $max_retries ]; do
    echo "Retry $i failed. Waiting $retry_delay seconds before retrying..."
    sleep $retry_delay
    ((i++))
  done

  if [ $i -gt $max_retries ]; then
    echo "Download failed after $max_retries attempts."
    exit 1
  fi
}

installation(){
    echo "Installing sonobuoy version ${version}"
    if [ ! -d "my-sonobuoy-plugins" ]; 
    then
        echo "Cloning repo: https://github.com/phillipsj/my-sonobuoy-plugins.git"
        download_retry "${mixed_plugins_url}"
    fi
    wait
    echo "Downloading sonobouy installer..."
    if [[ $(command -v wget) ]]; then
        download_retry "wget -q ${sonobuoy_url} -O sonobuoy.tar.gz"
        wait
    elif [[ $(command -v curl) ]]; then
        download_retry "curl -s ${sonobuoy_url} --output sonobuoy.tar.gz"
        wait
    else
        echo "Unable to use wget or curl to download sonobuoy installer, consider a networking error or an under configured OS if this error persists"
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
