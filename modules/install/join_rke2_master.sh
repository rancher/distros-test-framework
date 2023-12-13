#!/bin/bash
# This script is used to join one or more nodes as servers to the first sertver
echo "$@"
# the following lines are to enable debug mode
set -x
PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

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
install_method=${11}
server_flags=${12}
rhel_username=${13}
rhel_password=${14}

create_config() {
  hostname=$(hostname -f)
  mkdir -p /etc/rancher/rke2
  cat << EOF >>/etc/rancher/rke2/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${fqdn}
server: https://${server_ip}:9345
token:  "${token}"
node-name: "${hostname}"
EOF
}

update_config() {
  if [ -n "$server_flags" ] && [[ "$server_flags" == *":"* ]]
  then
    echo -e "$server_flags" >> /etc/rancher/rke2/config.yaml
    if [[ "$server_flags" != *"cloud-provider-name"* ]]
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
}

subscription_manager() {
  if [ "$node_os" = "rhel" ]
  then
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
  fi
}

cis_setup() {
  if [ -n "$server_flags" ] && [[ "$server_flags" == *"cis"* ]]
  then
    if [[ "$node_os" == *"rhel"* ]] || [[ "$node_os" == *"centos"* ]] || [[ "$node_os" == *"oracle"* ]]
    then
      cp -f /usr/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
    else
      cp -f /usr/local/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
    fi
    systemctl restart systemd-sysctl
    useradd -r -c "etcd user" -s /sbin/nologin -M etcd -U
  fi
}

install() {
  export "$install_mode"="$version"
  if [ -n "$install_method" ]
  then
    export INSTALL_RKE2_METHOD="$install_method"
  fi

  if [[ -z "$channel"  ]]
  then
    curl -sfL https://get.rke2.io | sh -
  else
    curl -sfL https://get.rke2.io | INSTALL_RKE2_CHANNEL="$channel" sh -
  fi
  
  sleep 10
  if [[ "$node_os" = *"rhel"* ]] || [[ "$node_os" = "centos8" ]] || [[ "$node_os" = *"oracle"* ]]; then
    yum install tar iptables -y
  fi
  cis_setup

  sudo systemctl enable rke2-server --now
}

path_setup() {
  cat << EOF >> .bashrc
export KUBECONFIG=/etc/rancher/rke2/rke2.yaml PATH=$PATH:/var/lib/rancher/rke2/bin:/opt/rke2/bin CRI_CONFIG_FILE=/var/lib/rancher/rke2/agent/etc/crictl.yaml && \
alias k=kubectl
EOF
  source .bashrc
}

main() {
  create_config
  update_config
  subscription_manager
  disable_cloud_setup
  install
}
main "$@"
