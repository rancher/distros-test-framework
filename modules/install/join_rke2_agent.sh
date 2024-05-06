#!/bin/bash
# This script is used to join one or more nodes as agents
echo "$@"
# the following lines are to enable debug mode
set -x
PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

# Script args
node_os=${1}
server_ip=${2}
token=${3}
public_ip=${4}
private_ip=${5}
ipv6_ip=${6}
install_mode=${7}
version=${8}
channel=${9}
install_method=${10}
worker_flags=${11}
rhel_username=${12}
rhel_password=${13}

create_config() {
  hostname=$(hostname -f)
  mkdir -p /etc/rancher/rke2
  cat << EOF >>/etc/rancher/rke2/config.yaml
server: https://${server_ip}:9345
token:  "${token}"
node-name: "${hostname}"
EOF
}

update_config() {
  if [ -n "$worker_flags" ] && [[ "$worker_flags" == *":"* ]]; then
    echo -e "$worker_flags" >> /etc/rancher/rke2/config.yaml
  fi

  if [[ "$worker_flags" != *"cloud-provider-name"* ]] || [[ -z "$worker_flags" ]]; then
    if [ -n "$ipv6_ip" ] && [ -n "$public_ip" ] && [ -n "$private_ip" ]; then
        echo -e "node-external-ip: $public_ip,$ipv6_ip" >> /etc/rancher/rke2/config.yaml
        echo -e "node-ip: $private_ip,$ipv6_ip" >> /etc/rancher/rke2/config.yaml
    elif [ -n "$ipv6_ip" ]; then
        echo -e "node-external-ip: $ipv6_ip" >> /etc/rancher/rke2/config.yaml
        echo -e "node-ip: $ipv6_ip" >> /etc/rancher/rke2/config.yaml
    else
        echo -e "node-external-ip: $public_ip" >> /etc/rancher/rke2/config.yaml
        echo -e "node-ip: $private_ip" >> /etc/rancher/rke2/config.yaml
    fi
  fi
    cat /etc/rancher/rke2/config.yaml
}

cis_setup() {
  if [ -n "$worker_flags" ] && [[ "$worker_flags" == *"cis"* ]]; then
    if [[ "$node_os" == *"rhel"* ]] || [[ "$node_os" == *"centos"* ]] || [[ "$node_os" == *"oracle"* ]]; then
      cp -f /usr/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
    else
      cp -f /usr/local/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
    fi
    systemctl restart systemd-sysctl
  fi
}

subscription_manager() {
  if [ "$node_os" = "rhel" ]; then
    subscription-manager register --auto-attach --username="$rhel_username" --password="$rhel_password" || echo "Failed to register or attach subscription."
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

    workaround="[keyfile]\nunmanaged-devices=interface-name:cali*;interface-name:tunl*;interface-name:vxlan.calico;interface-name:flannel*"
    if [ ! -e /etc/NetworkManager/conf.d/canal.conf ]; then
      echo -e "$workaround" > /etc/NetworkManager/conf.d/canal.conf
    else
      echo -e "$workaround" >> /etc/NetworkManager/conf.d/canal.conf
    fi
    sudo systemctl reload NetworkManager
  fi
}

install() {
  export "$install_mode"="$version"
  if [ -n "$install_method" ]; then
    export INSTALL_RKE2_METHOD="$install_method"
  fi

  if [[ -z "$channel" ]]; then
    curl -sfL https://get.rke2.io | INSTALL_RKE2_TYPE='agent' sh -
  else
    curl -sfL https://get.rke2.io | INSTALL_RKE2_CHANNEL="$channel" INSTALL_RKE2_TYPE='agent' sh -
  fi

  sleep 10
  if [[ "$node_os" = *"rhel"* ]] || [[ "$node_os" = "centos8" ]] || [[ "$node_os" = *"oracle"* ]]; then
    yum install tar iptables -y
  fi
  cis_setup

  sudo systemctl enable rke2-agent --now
}

main() {
  create_config
  update_config
  subscription_manager
  disable_cloud_setup
  install
}
main "$@"
