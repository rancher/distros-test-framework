#!/bin/bash

## Uncomment the following lines to enable debug mode
# set -x
# echo "$@"

arch=$(uname -m)

has_bin() {
  bin="$(command -v "$1")"
  if [ -z "$bin" ]; then 
    echo "error: ${0} requires ${1} to use this function"
  fi
}

install_docker() {
  max_iteration=3
  install_cmd="curl -fsSL https://get.docker.com | sh"
  if ! eval "$install_cmd"; then
    echo "Failed to install docker on bastion node, Retrying..."
    for i in $(seq 1 $max_iteration); do
      eval "$install_cmd"
      result=$?
        if [[ $result -eq 0 ]]; then
          echo "Retry successful!"
          break
        else
          echo "Retrying..."
          sleep 2
          ((i++))
        fi
    done
  fi

  if [[ $result -ne 0 ]]; then
    echo "Failed to install docker on bastion node!!! Delete the instance and try again"
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

main() {
  has_bin docker
  if [[ "$(command)" =~ "error" ]] || [ -z "$(command)" ]; then
    install_docker
  fi
}
main "$@"
