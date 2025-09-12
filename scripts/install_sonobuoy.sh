#!/bin/bash

# Usage: sh install_sonobuoy.sh runs with default values
# Usage: sh install_sonobouy.sh install 0.56.17 arm64
# Usage: sh install_sonobouy.sh delete

action=${1:-install}
version=${2:-0.57.3}
arch=${3:-amd64}
mixed_plugins_url="git clone https://github.com/phillipsj/my-sonobuoy-plugins.git"
sonobuoy_url="https://github.com/vmware-tanzu/sonobuoy/releases/download/v${version}/sonobuoy_${version}_linux_${arch}.tar.gz"
max_retries=5
retry_delay=11
# adopt golang error handling in bash check variables are passed in appropriately - if not return appropriate error message

get_binpath(){
  # Default location for non-root user
  binpath=~/bin

  # /usr/local/bin is only accessible if we are root
  if (( $(id -u) == 0 )); then
    # Default location for root user
    binpath=/usr/local/bin

    # Check if /usr/local/bin is on a read-pnly filesystem, use ~/bin if so
    realdir=$(findmnt -n -o SOURCE --target ${binpath})

    # This line is mainly here to support LVM-based configuration
    realdir=$(sed 's/.*\[\/@\(.*\)\]/\1/' <<<${realdir})

	# Use "homed" bin location if /usr is read-only
    mountperm=$(findmnt -n -o OPTIONS ${realdir} | cut -d, -f1)
    [[ "${mountperm}" == "ro" ]] && binpath=~/bin
  fi

  # To be sure that the path is available (it does not hurt to do it)
  mkdir -p ${binpath}

  echo ${binpath}
}

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
    mv sonobuoy $(get_binpath)/sonobuoy
    chmod +x $(get_binpath)/sonobuoy
}

deletion(){
    echo "Deleting sonobuoy installer"
    rm -rf my-sonobuoy-plugins
    rm -rf sonobuoy_*
    rm -rf $(get_binpath)/sonobuoy
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
