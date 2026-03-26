#!/bin/bash

## Uncomment the following lines to enable debug mode
# set -x
# echo "$@"

set -e
exec > >(tee -a bastion_prep.log) 2>&1

arch=$(uname -m)

has_bin() {
  bin="$(command -v "$1")"
  if [ "$bin" == "" ]; then
    echo "error: ${1} is not found."
  else
    echo "$bin"
  fi
}

install_docker() {
  max_attempt=4
  delay=5
  # Download and execute an install script with basic validation.
  safe_install() {
      url="$1"
      shift
      env_vars=()
      extra_args=()
      found_separator=false

      for arg in "$@"; do
          if [ "$arg" = "--" ]; then
              found_separator=true
              continue
          fi
          if $found_separator; then
              extra_args+=("$arg")
          else
              env_vars+=("$arg")
          fi
      done

      tmp_script=$(mktemp /tmp/install-XXXXXX.sh) || return 1
      echo "Downloading install script from: $url"
      if ! curl -fsSL "$url" -o "$tmp_script"; then
          echo "ERROR: Failed to download from $url"
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
          echo "ERROR: Not a shell script"
          rm -f "$tmp_script"
          return 1
      fi
      if grep -qiE '(base64 -d|/dev/tcp/|nc -e)' "$tmp_script"; then
          echo "ERROR: Suspicious patterns"
          rm -f "$tmp_script"
          return 1
      fi

      echo "Validation passed. Executing install script..."
      env "${env_vars[@]}" sh "$tmp_script" "${extra_args[@]}"
      exit_code=$?
      rm -f "$tmp_script"

      return $exit_code
  }
  if ! safe_install "https://get.docker.com"; then
    echo "Unable to install docker on node, Attempting retry..."
    for i in $(seq 1 $max_attempt); do
      safe_install "https://get.docker.com"
      result=$?
      echo "$result"
        if [ "$result" == "" ]; then
          echo "Retry successful!"
          break
        else
          echo "Retry attempt: $i after $delay seconds..."
          sleep $delay
          ((i++))
        fi
    done
    echo "Retry attempt reached max_attempt: $max_attempt"
    echo "Failed to install docker on node! Please try to install manually..."
  fi
}

install_kubectl() {
  KUBECTL_VERSION="v1.35.3"
  if [ "$arch" = "aarch64" ]; then
      KUBE_ARCH="arm64"
      KUBECTL_SHA256="6f0cd088a82dde5d5807122056069e2fac4ed447cc518efc055547ae46525f14"
  else
      KUBE_ARCH="amd64"
      KUBECTL_SHA256="fd31c7d7129260e608f6faf92d5984c3267ad0b5ead3bced2fe125686e286ad6"
  fi
  curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${KUBE_ARCH}/kubectl" && \
  echo "${KUBECTL_SHA256}  kubectl" | sha256sum -c - && \
  chmod +x ./kubectl && \
  mv ./kubectl /usr/local/bin
}

install_podman() {
  [ -r /etc/os-release ] && . /etc/os-release
  if [ "$(expr "${ID_LIKE}" : ".*suse.*")" != 0 ]; then
    echo "Checking for package manager locks if so, removing them."
    pkill -f zypper 2>/dev/null || true
    rm -f /var/run/zypp.pid 2>/dev/null || true
    sleep 15

    echo "Installing podman using zypper..."
    zypper install -y podman
  else
    echo "Installing podman using apt-get..."
    apt-get -yq install podman
  fi
}

main() {
  echo "Install kubectl..."
  install_kubectl
  echo "Wait for 30 seconds for the process to finish"
  sleep 30
  has_docker=$(has_bin docker)
  if [[ "$has_docker" =~ "error" ]] || [ -z "$has_docker" ]; then
    echo "Install docker..."
    install_docker
    echo "Wait for 30 seconds for the process to finish"
    sleep 30
  else
    echo "Found docker in path: $has_docker"
  fi
  echo "Install podman..."
  install_podman
}
main "$@"
