#!/bin/bash

## Uncomment the following lines to enable debug mode
# set -x
# echo "$@"

# Usage: ./get_artifacts.sh k3s v1.27.5+k3s1
# Usage: ./get_artifacts.sh rke2 v1.27.5+rke2r1 amd64 "flags" tar.gz

product=$1
version=$2
arch=$3
flags=${4}
tarball_type=$5
prodbin=$product

check_arch(){
  if [[ -n "$arch" ]] && [[ "$arch" =~ "arm" ]]; then
    if [[ "$product" == "k3s" ]]; then
      prodbin="k3s-arm64"
    else
      arch="arm64"
    fi
  else
    arch="amd64"
  fi
}

get_assets() {
  echo "Downloading $product dependencies..."
  if [[ "$product" == "k3s" ]]; then
    url="https://github.com/k3s-io/k3s/releases/download/$version"
    wget "$url"/k3s-images.txt
    wget -O k3s-install.sh https://get.k3s.io/
    wget -O k3s "$url"/"$prodbin"
    if [ -n "$tarball_type" ]; then
      wget "$url"/k3s-airgap-images-"$arch"."$tarball_type"
    fi
  elif [[ "$product" == "rke2" ]]; then
    url="https://github.com/rancher/rke2/releases/download/$version"
    wget "$url"/sha256sum-"$arch".txt
    wget "$url"/rke2-images.linux-"$arch".txt
    wget "$url"/rke2.linux-"$arch".tar.gz
    wget -O rke2-install.sh https://get.rke2.io/
    if [ -n "$tarball_type" ]; then
      wget "$url"/rke2-images.linux-"$arch"."$tarball_type"
    fi
  else
    echo "Invalid product: $product. Please provide k3s or rke2 as product"
  fi
}

get_cni_assets() {
  if [[ -n "$flags" ]] && [[ "$flags" =~ "cni" ]]; then
    url="https://github.com/rancher/rke2/releases/download/$version"
    cnis=("calico" "canal" "cilium" "flannel" "multus")
    for cni in "${cnis[@]}"; do
      if [[ "$flags" =~ $cni ]]; then
        wget "$url"/rke2-images-"$cni".linux-"$arch".txt
        if [ -n "$tarball_type" ]; then
          wget "$url"/rke2-images-"$cni".linux-"$arch"."$tarball_type"
        fi
        break
      fi
    done
  fi
}

save_to_directory() {
  folder="$(pwd)/artifacts"
  echo "Saving $product dependencies in directory $folder..."
  sudo mkdir "$folder"
  sudo cp -r ./*linux* sha256sum-"$arch".txt "$folder"
}

main() {
  check_arch
  get_assets
  if [[ "$product" == "rke2" ]]; then
    get_cni_assets
    save_to_directory
  fi
}
main "$@"
