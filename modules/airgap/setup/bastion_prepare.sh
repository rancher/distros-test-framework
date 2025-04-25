#!/bin/bash

## Uncomment the following lines to enable debug mode
# set -x
# echo "$@"

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
  install_cmd="curl -fsSL https://get.docker.com | sh"
  if ! eval "$install_cmd"; then
    echo "Unable to install docker on node, Attempting retry..."
    for i in $(seq 1 $max_attempt); do
      eval "$install_cmd"
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
  if [ "$arch" = "aarch64" ]; then
      KUBE_ARCH="arm64"
  else
      KUBE_ARCH="amd64"
  fi
  curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/$KUBE_ARCH/kubectl" && \
  chmod +x ./kubectl && \
  mv ./kubectl /usr/local/bin
}

install_podman() {
  [ -r /etc/os-release ] && . /etc/os-release
  if [ `expr "${ID_LIKE}" : ".*suse.*"` != 0 ]; then
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
