#!/bin/bash

## Uncomment the following lines to enable debug mode
#set -x
# PS4='+(${LINENO}): '

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
  sudo mkdir -p /etc/rancher/$product
  cat << EOF >> /etc/rancher/$product/config.yaml
node-name: $hostname
EOF
}

add_config() {
  if [ -n "$flags" ]; then
    echo "$flags" >> /etc/rancher/$product/config.yaml
  fi

  if [ "$flags" != *"cloud-provider-name"* ]; then
    if [ -n "$ipv6_ip" ] && [ -n "$private_ip" ]; then
      echo "node-ip: $private_ip,$ipv6_ip" >> /etc/rancher/$product/config.yaml
    elif [ -n "$ipv6_ip" ]; then
      echo "node-ip: $ipv6_ip" >> /etc/rancher/$product/config.yaml
      server_ip="[$server_ip]"
    else
      echo "node-ip: $private_ip" >> /etc/rancher/$product/config.yaml
    fi
  fi
  cat /etc/rancher/$product/config.yaml
}

install() {
  if [ "$product" = "k3s" ]; then
    if [ "$node_type" = "server" ]; then
      INSTALL_K3S_SKIP_DOWNLOAD=true ./k3s-install.sh --write-kubeconfig-mode 644 --cluster-init
      sleep 30
    elif [ "$node_type" = "agent" ]; then
      INSTALL_K3S_SKIP_DOWNLOAD=true K3S_URL="https://$server_ip:6443" K3S_TOKEN="$token" ./k3s-install.sh
      sleep 30
    else
      echo "Invalid type. Expected type to be server or agent, found $type!"
    fi
  elif [ "$product" = "rke2" ]; then
    if [ "$node_type" = "server" ]; then
      sudo rke2 server --write-kubeconfig-mode 644 > /dev/null 2>&1 &
      sleep 60
    elif [ "$node_type" = "agent" ]; then
      sudo rke2 agent --server "https://$server_ip:9345" --token "$token" > /dev/null 2>&1 &
      sleep 60
    else
      echo "Invalid type. Expected type to be server or agent, found $type!"
    fi
    sudo chmod 644 /etc/rancher/$product/$product.yaml
    cat << EOF >> .bashrc
export KUBECONFIG=/etc/rancher/$product/$product.yaml PATH=$PATH:/var/lib/rancher/$product/bin:/opt/$product/bin && \
alias k=kubectl
EOF
source .bashrc
  else
    echo "Invalid product. Expected product to be k3s or rke2, found $product!"
  fi
}

wait_nodes() {
  export PATH="$PATH:/usr/local/bin"
  timeElapsed=0

  while (( timeElapsed < 300 )); do
    if kubectl get nodes --kubeconfig=/etc/rancher/$product/$product.yaml >/dev/null 2>&1; then
      return
    fi
    sleep 5
    ((timeElapsed += 5))
  done

  echo "Timed out while waiting for nodes."
  exit 1
}

wait_ready_nodes() {
  IFS=$'\n'
  timeElapsed=0
  sleep 10

  while (( timeElapsed < 420 )); do
    not_ready=false
    for rec in $(kubectl get nodes --kubeconfig=/etc/rancher/$product/$product.yaml); do
      if [[ "$rec" == *"NotReady"* ]]; then
        not_ready=true
        break
      fi
    done
    if [[ $not_ready == false ]]; then
      return
    fi
    sleep 20
    ((timeElapsed += 20))
  done

  echo "Timed out while waiting for ready nodes."
  exit 1
}

config_files() {
  sudo cat /etc/rancher/$product/config.yaml > /tmp/joinflags
  sudo cat /var/lib/rancher/$product/server/node-token > /tmp/nodetoken
  sudo cat /etc/rancher/$product/$product.yaml > /tmp/config
  cat << EOF >> .bashrc
export KUBECONFIG=/etc/rancher/$product/$product.yaml PATH=$PATH:/var/lib/rancher/$product/bin:/opt/$product/bin && \
alias k=kubectl
EOF
source .bashrc
}

main() {
  create_config
  add_config
  install
  if [ "$node_type" = "server" ]; then
    config_files
    wait_nodes
    wait_ready_nodes
  fi
  
}
main "$@"