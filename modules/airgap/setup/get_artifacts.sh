#!/bin/bash

## Uncomment the following lines to enable debug mode
#set -x
# PS4='+(${LINENO}): '

set -x
echo "$@"

# Usage: ./download_product.sh k3s v1.27.5+k3s1
# Usage: ./download_product.sh rke2 v1.27.5+rke2 amd64 tar.gz

product=$1
version=$2
arch=$3
tarball_type=$4
prodbin=$product
artifacts=$5
flags=$6
os=$7

check_arch(){
  if [[ -n "$arch" ]] && [ "$arch" = *"arm"* ]
  then
    if [[ "$product" == "k3s" ]]
    then
      prodbin="k3s-arm64"
    else
      arch="arm64"
    fi
  else
    arch="amd64"
  fi
}

check_tar(){
  if [[ -z $tarball_type ]]; then
    tarball_type="tar.gz"
  fi
}


get_assets() {
  echo "Downloading $product dependencies..."
  if [[ "$product" == "k3s" ]]; then
    wget -O k3s-images.txt https://github.com/k3s-io/k3s/releases/download/$version/k3s-images.txt
    wget -O k3s-install.sh https://get.k3s.io/
    wget -O k3s https://github.com/k3s-io/k3s/releases/download/$version/$prodbin
  elif [[ "$product" == "rke2" ]]; then
    wget -O sha256sum-$arch.txt https://github.com/rancher/rke2/releases/download/$version/sha256sum-$arch.txt
    wget -O rke2-images.txt https://github.com/rancher/rke2/releases/download/$version/rke2-images.linux-$arch.txt
    #wget -O rke2-images.linux-$arch.$tarball_type https://github.com/rancher/rke2/releases/download/$version/rke2-images.linux-$arch.$tarball_type
    wget -O rke2.linux-$arch.tar.gz https://github.com/rancher/rke2/releases/download/$version/rke2.linux-$arch.tar.gz
    #wget -O rke2 https://github.com/rancher/rke2/releases/download/$version/rke2.linux-$arch
    wget -O rke2-install.sh https://get.rke2.io/
  else
    echo "Invalid product: $product. Please provide k3s or rke2 as product"
  fi
  sleep 2
}

validate_assets() {
  echo "Checking $product dependencies downloads locally... "
  if [[ ! -f "$product-images.txt" ]]
  then
    echo "$product-images.txt file not found!"
  fi
  if [[ ! -f "$product" ]]
  then
    echo "$product directory not found!"
  fi
  if [[ ! -f "$product-install.sh" ]]
  then
    echo "$product-install.sh file not found!"
  fi
  
}

save_to_directory() {
  folder="`pwd`/artifacts"
  echo "Saving $product dependencies in directory $folder..."
  sudo mkdir $folder
  sudo cp -r $product* sha256sum-$arch.txt $folder
}

main() {
  check_arch
  check_tar
  get_assets
  validate_assets
  if [[ "$product" == "rke2" ]]; then
    save_to_directory
  fi
  sleep 5
}
main "$@"