#!/bin/bash

## Uncomment the following lines to enable debug mode
#set -x
# PS4='+(${LINENO}): '
# ./install_product.sh product serverIP token nodeType privateIP ipv6IP flags

set -x
echo "$@"

product=${1}
server_ip=${2}
token=${3}
node_type=${4}
private_ip=${5}
ipv6_ip=${6}
flags=${7}

create_config() {
  hostname=$(hostname -f)
  mkdir -p /etc/rancher/$product
  cat <<EOF >>/etc/rancher/$product/config.yaml
node-name: $hostname
EOF
}

add_to_config() {
  if [[ "$node_type" == "server" ]]; then
    echo "write-kubeconfig-mode: 644" >>/etc/rancher/$product/config.yaml
    if [[ "$product" == "k3s" ]] && [[ -z "$server_ip" ]]; then
      echo "cluster-init: true" >>/etc/rancher/$product/config.yaml
    fi
  fi

  if [ -n "$server_ip" ]; then
    if [[ "$product" == "k3s" ]]; then
      echo "server: 'https://$server_ip:6443'" >>/etc/rancher/$product/config.yaml
    elif [[ "$product" == "rke2" ]]; then
      echo "server: 'https://$server_ip:9345'" >>/etc/rancher/$product/config.yaml
    else
      echo "Invalid product $product"
    fi
    echo "token: '$token'" >>/etc/rancher/$product/config.yaml
  fi

  if [ -n "$flags" ]; then
    echo "$flags" >>/etc/rancher/$product/config.yaml
  fi

  if [ "$flags" != *"cloud-provider-name"* ]; then
    if [ -n "$ipv6_ip" ] && [ -n "$private_ip" ]; then
      echo "node-ip: $private_ip,$ipv6_ip" >>/etc/rancher/$product/config.yaml
    elif [ -n "$ipv6_ip" ]; then
      echo "node-ip: $ipv6_ip" >>/etc/rancher/$product/config.yaml
      server_ip="[$server_ip]"
    else
      echo "node-ip: $private_ip" >>/etc/rancher/$product/config.yaml
    fi
  fi
  cat /etc/rancher/$product/config.yaml
}

install() {
  if [ "$product" = "k3s" ]; then
    if [ "$node_type" = "server" ]; then
      INSTALL_K3S_SKIP_DOWNLOAD=true ./k3s-install.sh
      sleep 60
    elif [ "$node_type" = "agent" ]; then
      INSTALL_K3S_SKIP_DOWNLOAD=true K3S_URL="https://$server_ip:6443" K3S_TOKEN="$token" ./k3s-install.sh
      sleep 30
    else
      echo "Invalid type. Expected type to be server or agent, found $type!"
    fi
  elif [ "$product" = "rke2" ]; then
    if [ "$node_type" = "server" ]; then
      INSTALL_RKE2_ARTIFACT_PATH="`pwd`/artifacts" ./rke2-install.sh
      systemctl enable rke2-server.service --now
      sleep 180
    elif [ "$node_type" = "agent" ]; then
      INSTALL_RKE2_ARTIFACT_PATH="`pwd`/artifacts" INSTALL_RKE2_TYPE="agent" ./rke2-install.sh
      systemctl enable rke2-agent.service --now
      sleep 90
    else
      echo "Invalid type. Expected type to be server or agent, found $type!"
    fi
  else
    echo "Invalid product. Expected product to be k3s or rke2, found $product!"
  fi
}

# TODO
# install_rpm() {

# }

main() {
  create_config
  add_to_config
  install
}

main "$@"