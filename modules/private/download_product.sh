#!/bin/bash

## Uncomment the following lines to enable debug mode
#set -x
# PS4='+(${LINENO}): '

set -x
echo "$@"

# Usage: ./download_product.sh k3s v1.27.5+k3s1

product=$1
version=$2
arch=$3
prodbin=$product

check_arch(){
  if [[ -n "$arch" ]] && [[ "$arch" == *"arm"* ]]
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


download_product() {
  echo "Downloading $product dependencies..."
  if [[ "$product" == "k3s" ]]
  then
    wget -O k3s-images.txt https://github.com/k3s-io/k3s/releases/download/$version/k3s-images.txt
    wget -O k3s-install.sh https://get.k3s.io/
    wget -O k3s https://github.com/k3s-io/k3s/releases/download/$version/$prodbin
  elif [[ "$product" == "rke2" ]]
  then
    wget -O rke2-images.txt https://github.com/rancher/rke2/releases/download/$version/rke2-images-all.linux-amd64.txt
    wget -O rke2 https://github.com/rancher/rke2/releases/download/$version/rke2.linux-$arch
  else
    echo "Invalid product: $product. Please provide k3s or rke2 as product"
  fi
  sleep 2
}

validate_download() {
  echo "Checking $product dependencies downloads locally... "
  if [[ ! -f "$product-images.txt" ]]
  then
    echo "$product-images.txt file not found!"
  fi
  if [[ ! -f "$product" ]]
  then
    echo "$product directory not found!"
  fi
  if [[ "$product" == "k3s" ]]
  then
    if [[ ! -f "k3s-install.sh" ]]
    then
      echo "k3s-install.sh file not found!"
    fi
  fi
}

save_to_directory() {
  folder="/tmp/"$product"-assets"
  echo "Saving $product dependencies in directory $folder..."
  sudo mkdir $folder
  sudo cp -r $product* $folder
  sudo rm -rf $product*
}

main() {
  check_arch
  download_product
  validate_download
  save_to_directory
}
main "$@"