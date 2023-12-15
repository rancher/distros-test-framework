#!/bin/bash

# the following lines are to enable debug mode
set -x
PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

# Script args
node_os=${1}
fqdn=${2}
server_ip=${3}
token=${4}
public_ip=${5}
private_ip=${6}
ipv6_ip=${7}
install_mode=${8}
version=${9}
channel=${10}
datastore_type=${11}
datastore_endpoint=${12}
server_flags=${13}
rhel_username=${14}
rhel_password=${15}

create_config() {
  hostname=$(hostname -f)
  sudo mkdir -p /etc/rancher/k3s
  cat <<EOF >>/etc/rancher/k3s/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${fqdn}
server: https://${server_ip}:6443
token: "${token}"
node-name: $hostname
EOF
}

update_config() {
  if [ -n "$server_flags" ] && [[ "$server_flags" == *":"* ]]; then
    echo -e "$server_flags" >> /etc/rancher/k3s/config.yaml
  fi

  if [[ "$server_flags" != *"cloud-provider-name"* ]]; then
    if [ -n "$ipv6_ip" ] && [ -n "$public_ip" ] && [ -n "$private_ip" ]; then
      echo -e "node-external-ip: $public_ip,$ipv6_ip" >> /etc/rancher/k3s/config.yaml
      echo -e "node-ip: $private_ip,$ipv6_ip" >> /etc/rancher/k3s/config.yaml
    elif [ -n "$ipv6_ip" ]; then
      echo -e "node-external-ip: $ipv6_ip" >> /etc/rancher/k3s/config.yaml
      echo -e "node-ip: $ipv6_ip" >> /etc/rancher/k3s/config.yaml
    else
      echo -e "node-external-ip: $public_ip" >> /etc/rancher/k3s/config.yaml
      echo -e "node-ip: $private_ip" >> /etc/rancher/k3s/config.yaml
    fi
  fi
  cat /etc/rancher/k3s/config.yaml
}

subscription_manager() {
   if [ "$node_os" = "rhel" ]; then
    subscription-manager register --auto-attach --username="$username" --password="$password" || echo "Failed to register or attach subscription."
    subscription-manager repos --enable=rhel-7-server-extras-rpms || echo "Failed to enable repositories."
   fi
}

disable_cloud_setup() {
  if [[ "$node_os" = *"rhel"* ]] || [[ "$node_os" = "centos8" ]] || [[ "$node_os" = *"oracle"* ]]; then
    if systemctl is-enabled --quiet nm-cloud-setup.service 2>/dev/null; then
      systemctl disable nm-cloud-setup.service
    else
      echo "nm-cloud-setup.service not found or not enabled"
    fi

    if systemctl is-enabled --quiet nm-cloud-setup.timer 2>/dev/null; then
      systemctl disable nm-cloud-setup.timer
    else
      echo "nm-cloud-setup.timer not found or not enabled"
    fi
  fi
}

policy_files() {
  if [[ -n "$server_flags"  ]] && [[ "$server_flags"  == *"protect-kernel-defaults"* ]]; then
    sudo mkdir -p -m 700 /var/lib/rancher/k3s/server/logs
    sudo mkdir -p /var/lib/rancher/k3s/server/manifests
    cat /tmp/cis_master_config.yaml >> /etc/rancher/k3s/config.yaml
    printf "%s\n" "vm.panic_on_oom=0" "vm.overcommit_memory=1" "kernel.panic=10" "kernel.panic_on_oops=1" "kernel.keys.root_maxbytes=25000000" >> /etc/sysctl.d/90-kubelet.conf
    sysctl -p /etc/sysctl.d/90-kubelet.conf
    systemctl restart systemd-sysctl
    cat /tmp/policy.yaml > /var/lib/rancher/k3s/server/manifests/policy.yaml
    cat /tmp/audit.yaml > /var/lib/rancher/k3s/server/audit.yaml
    cat /tmp/cluster-level-pss.yaml > /var/lib/rancher/k3s/server/cluster-level-pss.yaml
    cat /tmp/ingresspolicy.yaml > /var/lib/rancher/k3s/server/manifests/ingresspolicy.yaml
  fi
  sleep 20
}

install() {
  export "$install_mode"="$version"

  if [ "$datastore_type" = "etcd" ]; then
    if [[ -n "$channel" ]]; then
      curl -sfL https://get.k3s.io | INSTALL_K3S_CHANNEL=$channel INSTALL_K3S_TYPE='server' sh -
    else
      curl -sfL https://get.k3s.io | INSTALL_K3S_TYPE='server' sh -
    fi
  else
    if [[ -n "$channel" ]]; then
      curl -sfL https://get.k3s.io | INSTALL_K3S_CHANNEL=$channel INSTALL_K3S_TYPE='server' sh -s - server --datastore-endpoint="$datastore_endpoint"
    else
      curl -sfL https://get.k3s.io | INSTALL_K3S_TYPE='server' sh -s - server --datastore-endpoint="$datastore_endpoint"
    fi
  fi
  
  echo "alias k=kubectl" >> ~/.bashrc
  source ~/.bashrc
}

main() {
  create_config
  update_config
  policy_files
  subscription_manager
  disable_cloud_setup
  install
}
main "$@"