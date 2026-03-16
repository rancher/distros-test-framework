#!/bin/bash

## Uncomment the following lines to enable debug mode
set -x

exec 2> get_artifacts.log
# echo "$@"

# Usage: ./get_artifacts.sh "k3s" "v1.31.0+k3s1"
# Usage: ./get_artifacts.sh "rke2" "v1.31.0+rke2r1" "linux" "amd64" "server_flags" "tar.gz"

product=${1}
version=${2}
platform=${3}
arch=${4}
registry_url=${5}
server_flags=${6}
tarball_type=${7}
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

override_arch() {
  local os_arch
  os_arch=$(uname -m)
  echo "OS Architecture: $os_arch"

  case "$os_arch" in
    x86_64)
      arch="amd64"
      k3s_binary="k3s"
      ;;
    aarch64|arm64)
      arch="arm64"
      [[ "$product" == "k3s" ]] && k3s_binary="k3s-arm64"
      ;;
    armhf|armv7l)
      arch="arm"
      [[ "$product" == "k3s" ]] && k3s_binary="k3s-armhf"
      ;;
    *)
      echo "Error: Unsupported Architecture: $os_arch"
      return 1
      ;;
  esac
}

get_url() {
  local url=""
  if [[ -n "$registry_url" ]]; then
    if [[ "$registry_url" =~ "prime" ]]; then
      url="$registry_url/$product/$version"
    else
      echo "Error: Unsupported registry_url '$registry_url'. Only 'prime' registries are currently supported." >&2
      exit 1
    fi
  else
    if [[ "$product" == "k3s" ]]; then
      url="https://github.com/k3s-io/k3s/releases/download/$version"
    elif [[ "$product" == "rke2" ]]; then
      url="https://github.com/rancher/rke2/releases/download/$version"
    fi
  fi
  echo "$url"
}

download_retry() {
  max_attempts=3
  attempt_num=1

  while [[ $attempt_num -le $max_attempts ]]; do
    if "$@"; then
      echo "Command succeeded after $attempt_num attempts."
      break
    else
      echo "Attempt $attempt_num failed. Retrying in 5 seconds..."
      attempt_num=$((attempt_num + 1))
      sleep 5
    fi
  done

  if [[ $attempt_num -gt $max_attempts ]]; then
    echo "ERROR: Command failed after $max_attempts attempts."
    return 1
  fi
}

# Verify downloaded files against a sha256sum file.
# Usage: verify_checksums <checksums_file> [file1 file2 ...]
# If no files specified, verifies all files listed in the checksums file that exist locally.
verify_checksums() {
  checksums_file="$1"
  shift

  if [ ! -f "$checksums_file" ]; then
    echo "ERROR: Checksums file not found: $checksums_file — aborting verification"
    return 1
  fi

  if [ $# -gt 0 ]; then
    # Verify specific files
    for file in "$@"; do
      basename=$(basename "$file")
      if grep -q "$basename" "$checksums_file"; then
        if grep "$basename" "$checksums_file" | sha256sum -c -; then
          echo "Checksum OK: $basename"
        else
          echo "ERROR: Checksum FAILED for $basename"
          return 1
        fi
      fi
    done
  else

    # Verify all files in checksums that exist locally
    while IFS= read -r line; do
      file=$(echo "$line" | awk '{print $2}' | sed 's/^\*//')
      if [ -f "$file" ]; then
        if echo "$line" | sha256sum -c -; then
          echo "Checksum OK: $file"
        else
          echo "ERROR: Checksum FAILED for $file"
          return 1
        fi
      fi
    done < "$checksums_file"
  fi
}

# Safely download an install — download first, validate, then save.
safe_download_install() {
  url="$1"
  output="$2"
  tmp_script=$(mktemp /tmp/install-XXXXXX.sh)

  if ! wget -qO "$tmp_script" "$url"; then
    echo "ERROR: Failed to download install script from $url"
    rm -f "$tmp_script"

    return 1
  fi

  if [ ! -s "$tmp_script" ]; then
    echo "ERROR: Downloaded script is empty"
    rm -f "$tmp_script"

    return 1
  fi

  first_line=$(head -1 "$tmp_script")
  if ! echo "$first_line" | grep -qE '^#!\s*/(bin|usr/bin)/(sh|bash|env\s+(sh|bash))'; then
    echo "ERROR: Downloaded file does not appear to be a shell script (first line: $first_line)"
    rm -f "$tmp_script"

    return 1
  fi

  if grep -qiE '(base64 -d|/dev/tcp/|nc -e|eval.*\$\(curl)' "$tmp_script"; then
    echo "ERROR: Suspicious patterns detected in downloaded script from $url"
    rm -f "$tmp_script"

    return 1
  fi

  mv "$tmp_script" "$output"
  echo "Install script validated and saved: $output"
}

get_assets() {
  echo "Downloading $product dependencies..."
  if [[ "$product" == "k3s" ]]; then
    url=$(get_url)
    download_retry wget $url/sha256sum-$arch.txt
    download_retry wget $url/k3s-images.txt
    safe_download_install "https://get.k3s.io/" "k3s-install.sh"
    download_retry wget -O $k3s_binary $url/$k3s_binary
    if [ -n "$tarball_type" ]; then
      download_retry wget $url/k3s-airgap-images-$arch.$tarball_type
    fi
    echo "Verifying K3s artifact checksums..."
    verify_checksums "sha256sum-$arch.txt"
  elif [[ "$product" == "rke2" ]]; then
    url=$(get_url)
    echo "Download assets using url: $url"
    download_retry wget $url/sha256sum-$arch.txt
    # Ref: https://docs.rke2.io/install/airgap
    if [[ -n "$server_flags" ]] && [[ "$server_flags" =~ "cni" ]]; then
      download_retry wget $url/rke2-images-core.linux-$arch.txt
      if [ -n "$tarball_type" ]; then
        download_retry wget $url/rke2-images-core.linux-$arch.$tarball_type
      fi
    elif [[ -z "$server_flags" ]] || [[ "$server_flags" != *"cni"* ]]; then
      download_retry wget $url/rke2-images.linux-$arch.txt
      if [ -n "$tarball_type" ]; then
        download_retry wget $url/rke2-images.linux-$arch.$tarball_type
      fi
    fi
    download_retry wget $url/rke2.linux-$arch.tar.gz
    safe_download_install "https://get.rke2.io/" "rke2-install.sh"
    echo "Verifying RKE2 artifact checksums..."
    verify_checksums "sha256sum-$arch.txt"
  else
    echo "Invalid product: $product. Please provide k3s or rke2 as product"
  fi
}

get_cni_assets() {
  if [[ -n "$server_flags" ]] && [[ "$server_flags" =~ "cni" ]] && [[ "$server_flags" != *"cni: none"* ]]; then
    url=$(get_url)
    echo "Download cni assets using url: $url"
    cnis=("calico" "canal" "cilium" "flannel")
    for cni in "${cnis[@]}"; do
      if [[ "$server_flags" =~ $cni ]]; then
        download_retry wget $url/rke2-images-$cni.linux-$arch.txt
        if [ -n "$tarball_type" ]; then
          download_retry wget $url/rke2-images-$cni.linux-$arch.$tarball_type
        fi
        break
      fi
    done
    if [[ "$server_flags" =~ "multus" ]]; then
      download_retry wget $url/rke2-images-multus.linux-$arch.txt
      if [ -n "$tarball_type" ]; then
        download_retry wget $url/rke2-images-multus.linux-$arch.$tarball_type
      fi
    fi
  fi
}

# TODO: Add function for ingress-controller: traefik

get_windows_assets() {
  url=$(get_url)
  echo "Download Windows assets using url: $url"
  download_retry wget $url/sha256sum-amd64.txt
  download_retry wget $url/rke2-images.windows-amd64.txt
  download_retry wget $url/rke2.windows-amd64.tar.gz
  download_retry wget -O rke2-install.ps1 https://raw.githubusercontent.com/rancher/rke2/master/install.ps1
  if [ -n "$tarball_type" ]; then
    download_retry wget $url/rke2-windows-ltsc2022-amd64-images.$tarball_type
  fi
  echo "Verifying Windows artifact checksums..."
  verify_checksums "sha256sum-amd64.txt"
  # TODO: Add logic for Win 2019 - rke2-windows-1809-amd64-images.$tarball_type
}

save_to_directory() {
  folder="$(pwd)/artifacts"
  if [[ "${1}" == "windows" ]]; then
    folder="$folder-windows"
    echo "Saving $product dependencies in directory $folder..."
    sudo mkdir "$folder"
    sudo cp -r ./*windows-* sha256sum-amd64.txt "$folder"
  else
    echo "Saving $product dependencies in directory $folder..."
    sudo mkdir "$folder"
    sudo cp -r ./*linux-* sha256sum-"$arch".txt "$folder"
  fi
}

main() {
  validate_args
  override_arch
  if [[ "$platform" == "windows" ]]; then
    get_windows_assets
    save_to_directory "windows"
  else
    get_assets
    if [[ "$product" == "rke2" ]]; then
      get_cni_assets
      save_to_directory
    fi
  fi
}

main "$@"
