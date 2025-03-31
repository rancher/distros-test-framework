#!/bin/bash
# This script is used to join one or more nodes as servers to the first server
# Usage:
# node_os=${1} # Node OS values. Ex: rhel8, centos8, slemicro
# fqdn=${2} # FQDN
# server_ip=${3} # Master Server IP to join to. Value will be added to config.yaml file.
# token=${4} # Token
# public_ip=${5} # Public IP of the joining server node
# private_ip=${6} # Private IP of the joining server node
# ipv6_ip=${7} # IPv6 IP of the joining server node
# install_mode=${8} # Install mode - INSTALL_<K3S|RKE2>_<VERSION|COMMIT>
# version=${9} # Version or Commit to Install
# channel=${10} # Channel to install from - testing latest or stable
# install_method=${11} # Method of install - rpm or tar
# datastore_type=${12} # Datastore type - etcd or external
# datastore_endpoint=${13} # Datastore Endpoint
# server_flags=${14} # Server Flags to add in config.yaml
# rhel_username=${15} # Rhel username
# rhel_password=${16} # Rhel password
# install_or_enable=${17}  # Values can be install, enable or both. In case of slemicro for node_os value, the first time this script is called with 'install'.
# After a node reboot, the second time the script is recalled with 'enable' which enables services.
# For all other node_os values, this value will be 'both' and this script will be called only once.
# set -x # Use for debugging script. Use 'set +x' to turn off debugging at a later stage, if needed.

echo "$@"

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
install_method=${11}
datastore_type=${12}
datastore_endpoint=${13}
server_flags=${14}
rhel_username=${15}
rhel_password=${16}
install_or_enable=${17}

create_config() {
  hostname=$(hostname -f)
  mkdir -p /etc/rancher/rke2
  cat <<EOF >>/etc/rancher/rke2/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${fqdn}
token:  "${token}"
EOF
}

update_config() {
  if [ -n "$server_flags" ] && [[ "$server_flags" == *":"* ]]; then
    echo -e "$server_flags" >>/etc/rancher/rke2/config.yaml
  fi

  if [[ "$server_flags" != *"cloud-provider-name"* ]] || [[ -z "$server_flags" ]]; then
    if [ -n "$ipv6_ip" ] && [ -n "$public_ip" ] && [ -n "$private_ip" ]; then
      echo -e "node-external-ip: $public_ip,$ipv6_ip" >>/etc/rancher/rke2/config.yaml
      echo -e "node-ip: $private_ip,$ipv6_ip" >>/etc/rancher/rke2/config.yaml
    elif [ -n "$ipv6_ip" ] && [ -z "$public_ip" ]; then
      echo -e "node-external-ip: $ipv6_ip" >>/etc/rancher/rke2/config.yaml
      server_ip="[$server_ip]"
      hostname="$hostname-srv$RANDOM"
    else
      echo -e "node-external-ip: $public_ip" >>/etc/rancher/rke2/config.yaml
      echo -e "node-ip: $private_ip" >>/etc/rancher/rke2/config.yaml
    fi
  fi
  echo -e server: https://"${server_ip}":9345 >>/etc/rancher/rke2/config.yaml
  echo -e node-name: "${hostname}" >>/etc/rancher/rke2/config.yaml

  if [ "$datastore_type" = "external" ]; then
    echo -e "datastore-endpoint: $datastore_endpoint" >>/etc/rancher/rke2/config.yaml
  fi
  cat /etc/rancher/rke2/config.yaml
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
      echo -e "$workaround" >/etc/NetworkManager/conf.d/canal.conf
    else
      echo -e "$workaround" >>/etc/NetworkManager/conf.d/canal.conf
    fi
  fi
}

cis_setup() {
  if [ -n "$server_flags" ] && [[ "$server_flags" == *"cis"* ]]; then
    if [[ "$node_os" == *"rhel"* ]] || [[ "$node_os" == *"centos"* ]] || [[ "$node_os" == *"oracle"* ]]; then
      cp -f /usr/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
    elif [[ "$node_os" == *"slemicro"* ]]; then
      groupadd --system etcd && useradd -s /sbin/nologin --system -g etcd etcd
      cat <<EOF >> ~/60-rke2-cis.conf
on_oovm.panic_on_oom=0
vm.overcommit_memory=1
kernel.panic=10
kernel.panic_ps=1
kernel.panic_on_oops=1
EOF
      cp ~/60-rke2-cis.conf /etc/sysctl.d/;
    else
      cp -f /usr/local/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
    fi
    systemctl restart systemd-sysctl
    if [[ "$node_os" != *"slemicro"* ]]; then
      useradd -r -c "etcd user" -s /sbin/nologin -M etcd -U
    fi
  fi
}

install_rke2() {
  url="https://get.rke2.io"
  params="$install_mode=$version INSTALL_RKE2_TYPE=server"
  
  if [ -n "$channel" ]; then
    params="$params INSTALL_RKE2_CHANNEL=$channel"
  fi

  if [ -n "$install_method" ]; then
    params="$params INSTALL_RKE2_METHOD=$install_method"
  fi

  install_cmd="curl -sfL $url | $params sh -"
  echo "$install_cmd"
  if ! eval "$install_cmd"; then
    echo "Failed to install rke2-server on joining node ip: $public_ip"
    exit 1
  fi
}

install_dependencies() {
  if [[ "$node_os" = *"rhel"* ]] || [[ "$node_os" = "centos8" ]] || [[ "$node_os" = *"oracle"* ]]; then
    yum install tar iptables -y
  fi
}

enable_service() {
  if ! sudo systemctl enable rke2-server --now; then
    echo "rke2-server failed to start on node: $public_ip, Waiting for 20s to retry..."

    ## rke2 can sometimes fail to start but some time after it starts successfully.
    sleep 20

    if ! sudo systemctl is-active --quiet rke2-server; then
      echo "rke2-server exiting after failed retry to start on node: $public_ip while joining server: $server_ip"
      sudo journalctl -xeu rke2-server.service --no-pager | grep -i "failed\|fatal"
      exit 1
    else
      echo "rke2-server started successfully on node: $public_ip"
    fi
  fi
}

install() {
  install_dependencies
  install_rke2
  sleep 10
  cis_setup
}

path_setup() {
  cat <<EOF >>.bashrc
export KUBECONFIG=/etc/rancher/rke2/rke2.yaml PATH=$PATH:/var/lib/rancher/rke2/bin:/opt/rke2/bin CRI_CONFIG_FILE=/var/lib/rancher/rke2/agent/etc/crictl.yaml && \
alias k=kubectl
EOF
  # shellcheck disable=SC1091
  source .bashrc
}

main() {
  echo "Install or enable or both? $install_or_enable"
  if [[ "${install_or_enable}" == "install" ]] || [[ "${install_or_enable}" == "both" ]]; then
    echo "Executing INSTALL Block"
    create_config
    update_config
    subscription_manager
    disable_cloud_setup
    install
    path_setup
  fi
  if [[ "${install_or_enable}" == "enable" ]] || [[ "${install_or_enable}" == "both" ]]; then
    echo "Executing ENABLE Block"
    enable_service
  fi
}
main "$@"
