#!/bin/bash

## Uncomment the following lines to enable debug mode
# set -x
# echo "$@"

# Usage: ./get_artifacts.sh "k3s" "v1.31.0+k3s1"
# Usage: ./get_artifacts.sh "rke2" "v1.31.0+rke2r1" "amd64" "server_flags" "tar.gz"

product=${1}
version=${2}
arch=${3}
server_flags=${4}
tarball_type=${5}
k3s_binary=$product

validate_args() {
  # Check product
  if [[ -z "$product" ]]; then
    echo "Product arg cannot be empty, must be either k3s or rke2!"
    exit 1
  elif [[ "$product" != "k3s" ]] && [[ "$product" != "rke2" ]]; then
    echo "Product is $product, must be k3s or rke2!"
    exit 1
  fi

  # Check version
  if [[ -z "$version" ]]; then
    echo "Version cannot be empty, please provide valid version!"
    exit 1
  elif [[ "$version" != *"k3s"* ]] && [[ "$version" != *"rke2"* ]]; then
    echo "Version cannot be commit ID, please provide valid version!"
    exit 1
  fi
}

check_arch() {
  if [[ -n "$arch" ]] && [[ "$arch" =~ "arm" ]]; then
    if [[ "$product" == "k3s" ]]; then
      k3s_binary="k3s-arm64"
    else
      arch="arm64"
    fi
  else
    arch="amd64"
  fi
}

download_retry() {
  cmd=${1}
  max_attempts=3
  attempt_num=1

  while [ $attempt_num -le $max_attempts ]; do
    if eval "$cmd"; then
      echo "Command succeeded after $attempt_num attempts."
      break
    else
      echo "Attempt $attempt_num failed. Retrying in 5 seconds..."
      attempt_num=$((attempt_num + 1))
      sleep 5
    fi
  done

  if [ $attempt_num -gt $max_attempts ]; then
    echo "Command failed after $max_attempts attempts."
  fi
}

get_assets() {
  echo "Downloading $product dependencies..."
  if [[ "$product" == "k3s" ]]; then
    url="https://github.com/k3s-io/k3s/releases/download/$version"
    download_retry "wget $url/k3s-images.txt"
    download_retry "wget -O k3s-install.sh https://get.k3s.io/"
    download_retry "wget -O k3s $url/$k3s_binary"
    if [ -n "$tarball_type" ]; then
      download_retry "wget $url/k3s-airgap-images-$arch.$tarball_type"
    fi
  elif [[ "$product" == "rke2" ]]; then
    url="https://github.com/rancher/rke2/releases/download/$version"
    download_retry "wget $url/sha256sum-$arch.txt"
    # Ref: https://docs.rke2.io/install/airgap
    if [[ -n "$server_flags" ]] && [[ "$server_flags" =~ "cni" ]]; then
      download_retry "wget $url/rke2-images-core.linux-$arch.txt"
      if [ -n "$tarball_type" ]; then
        download_retry "wget $url/rke2-images-core.linux-$arch.$tarball_type"
      fi
    elif [[ -z "$server_flags" ]]; then
      download_retry "wget $url/rke2-images.linux-$arch.txt"
      if [ -n "$tarball_type" ]; then
        download_retry "wget $url/rke2-images.linux-$arch.$tarball_type"
      fi
    fi
    download_retry "wget $url/rke2.linux-$arch.tar.gz"
    download_retry "wget -O rke2-install.sh https://get.rke2.io/"
  else
    echo "Invalid product: $product. Please provide k3s or rke2 as product"
  fi
}

get_cni_assets() {
  if [[ -n "$server_flags" ]] && [[ "$server_flags" =~ "cni" ]] && [[ "$server_flags" != *"cni: none"* ]]; then
    url="https://github.com/rancher/rke2/releases/download/$version"
    cnis=("calico" "canal" "cilium" "flannel")
    for cni in "${cnis[@]}"; do
      if [[ "$server_flags" =~ $cni ]]; then
        download_retry "wget $url/rke2-images-$cni.linux-$arch.txt"
        if [ -n "$tarball_type" ]; then
          download_retry "wget $url/rke2-images-$cni.linux-$arch.$tarball_type"
        fi
        break
      fi
    done
    if [[ "$server_flags" =~ "multus" ]]; then
      download_retry "wget $url/rke2-images-multus.linux-$arch.txt"
      if [ -n "$tarball_type" ]; then
        download_retry "wget $url/rke2-images-multus.linux-$arch.$tarball_type"
      fi
    fi
  fi
}

save_to_directory() {
  folder="$(pwd)/artifacts"
  echo "Saving $product dependencies in directory $folder..."
  sudo mkdir "$folder"
  sudo cp -r ./*linux* sha256sum-"$arch".txt "$folder"
}

main() {
  validate_args
  check_arch
  get_assets
  if [[ "$product" == "rke2" ]]; then
    get_cni_assets
    save_to_directory
  fi
}
main "$@"
