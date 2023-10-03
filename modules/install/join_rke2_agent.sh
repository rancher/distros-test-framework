#!/bin/bash
# This script is used to join one or more nodes as agents
set -x
echo "$@"

node_os=${1}
server_ip=${2}
token=${3}
public_ip=${4}
private_ip=${5}
ipv6_ip=${6}
version=${7}
channel=${8}
install_mode=${9}
install_method=${10}
worker_flags=${11}
rhel_username=${12}
rhel_password=${13}

hostname=$(hostname -f)
mkdir -p /etc/rancher/rke2
cat << EOF >/etc/rancher/rke2/config.yaml
server: https://${server_ip}:9345
token:  "${token}"
node-name: "${hostname}"
EOF

if [ -n "$worker_flags" ] && [[ "$worker_flags" == *":"* ]]
then
  echo -e "$worker_flags" >> /etc/rancher/rke2/config.yaml
  if [[ "$worker_flags" != *"cloud-provider-name"* ]]
  then
    if [ -n "$ipv6_ip" ] && [ -n "$public_ip" ] && [ -n "$private_ip" ]
    then
      echo -e "node-external-ip: $public_ip,$ipv6_ip" >> /etc/rancher/rke2/config.yaml
      echo -e "node-ip: $private_ip,$ipv6_ip" >> /etc/rancher/rke2/config.yaml
    elif [ -n "$ipv6_ip" ]
    then
      echo -e "node-external-ip: $ipv6_ip" >> /etc/rancher/rke2/config.yaml
      echo -e "node-ip: $ipv6_ip" >> /etc/rancher/rke2/config.yaml
    else
      echo -e "node-external-ip: $public_ip" >> /etc/rancher/rke2/config.yaml
      echo -e "node-ip: $private_ip" >> /etc/rancher/rke2/config.yaml
    fi
  fi
  cat /etc/rancher/rke2/config.yaml
fi

if [[ "$node_os" = "rhel" ]]
then
   subscription-manager register --auto-attach --username="$rhel_username" --password="$rhel_password"
   subscription-manager repos --enable=rhel-7-server-extras-rpms
fi

if [[ "$node_os" == *"rhel"* ]] || [[ "$node_os" == *"centos"* ]] || [[ "$node_os" == *"oracle"* ]]
then
  NM_CLOUD_SETUP_SERVICE_ENABLED=$(systemctl status nm-cloud-setup.service | grep -i enabled)
  NM_CLOUD_SETUP_TIMER_ENABLED=$(systemctl status nm-cloud-setup.timer | grep -i enabled)

  if [ "${NM_CLOUD_SETUP_SERVICE_ENABLED}" ]; then
    systemctl disable nm-cloud-setup.service
  fi

  if [ "${NM_CLOUD_SETUP_TIMER_ENABLED}" ]; then
    systemctl disable nm-cloud-setup.timer
  fi

  yum install tar -y
  yum install iptables -y
  workaround="[keyfile]\nunmanaged-devices=interface-name:cali*;interface-name:tunl*;interface-name:vxlan.calico;interface-name:flannel*"
  if [ ! -e /etc/NetworkManager/conf.d/canal.conf ]; then
    echo -e "$workaround" > /etc/NetworkManager/conf.d/canal.conf
  else
    echo -e "$workaround" >> /etc/NetworkManager/conf.d/canal.conf
  fi
  sudo systemctl reload NetworkManager
fi

export "$install_mode"="$version"

if [ -n "$install_method" ]
then
  export INSTALL_RKE2_METHOD="$install_method"
fi

if [ "$channel" != "null" ]
then
    curl -sfL https://get.rke2.io | INSTALL_channel="$channel" INSTALL_RKE2_TYPE='agent' sh -
else
    curl -sfL https://get.rke2.io | INSTALL_RKE2_TYPE='agent' sh -
fi
if [ -n "$worker_flags" ] && [[ "$worker_flags" == *"cis"* ]]
then
  if [[ "$node_os" == *"rhel"* ]] || [[ "$node_os" == *"centos"* ]] || [[ "$node_os" == *"oracle"* ]]
  then
    cp -f /usr/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
  else
    cp -f /usr/local/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
  fi
  systemctl restart systemd-sysctl
fi
sudo systemctl enable rke2-agent
sudo systemctl start rke2-agent