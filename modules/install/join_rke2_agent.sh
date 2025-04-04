#!/bin/bash
# This script is used to join one or more nodes as agents
# Usage:
# node_os=${1}              # Node OS values. Ex: rhel8, centos8, slemicro
# server_ip=${2}            # Master Server IP to join to. Value will be added to config.yaml file.
# token=${3}                # Node Token. Value will be added to config.yaml file.
# public_ip=${4}            # Public IP of the agent node. Value will be added to config.yaml file.
# private_ip=${5}           # Private IP of the agent node. Value will be added to config.yaml file.
# ipv6_ip=${6}              # IPv6 IP of the agent node. Value will be added to config.yaml file.
# install_mode=${7}         # Install mode - INSTALL_<K3S|RKE2>_<VERSION|COMMIT>
# version=${8}              # Version or Commit to install
# channel=${9}              # Channel to install from - testing, latest or stable
# install_method=${10}      # Method of install - rpm or tar
# worker_flags=${11}        # Worker flags to add in config.yaml file
# rhel_username=${12}       # rhel username
# rhel_password=${13}       # rhel password
# install_or_enable=${14}   # Values can be install, enable or both. In case of slemicro for node_os value, the first time this script is called with 'install'.
                            # After a node reboot, the second time the script is recalled with 'enable' which enables services.
                            # For all other node_os values, this value will be 'both' and this script will be called only once.
# set -x                    # Use for debugging script. Use 'set +x' to turn off debugging at a later stage, if needed.

echo "$@"

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
install_or_enable=${14}

create_config() {
  hostname=$(hostname -f)
  mkdir -p /etc/rancher/rke2
  cat <<EOF >>/etc/rancher/rke2/config.yaml
token:  "${token}"
EOF
}

update_config() {
  if [ -n "$worker_flags" ] && [[ "$worker_flags" == *":"* ]]; then
    echo -e "$worker_flags" >>/etc/rancher/rke2/config.yaml
  fi

  if [[ "$worker_flags" != *"cloud-provider-name"* ]] || [[ -z "$worker_flags" ]]; then
    if [ -n "$ipv6_ip" ] && [ -n "$public_ip" ] && [ -n "$private_ip" ]; then
      echo -e "node-external-ip: $public_ip,$ipv6_ip" >>/etc/rancher/rke2/config.yaml
      echo -e "node-ip: $private_ip,$ipv6_ip" >>/etc/rancher/rke2/config.yaml
    elif [ -n "$ipv6_ip" ] && [ -z "$public_ip" ]; then
      echo -e "node-external-ip: $ipv6_ip" >>/etc/rancher/rke2/config.yaml
      server_ip="[$server_ip]"
      hostname="$hostname-ag$RANDOM"
    else
      echo -e "node-external-ip: $public_ip" >>/etc/rancher/rke2/config.yaml
      echo -e "node-ip: $private_ip" >>/etc/rancher/rke2/config.yaml
    fi
  fi
  echo -e server: https://"${server_ip}":9345 >>/etc/rancher/rke2/config.yaml
  echo -e node-name: "${hostname}" >>/etc/rancher/rke2/config.yaml
  cat /etc/rancher/rke2/config.yaml
}

cis_setup() {
  if [ -n "$worker_flags" ] && [[ "$worker_flags" == *"cis"* ]]; then
    if [[ "$node_os" == *"rhel"* ]] || [[ "$node_os" == *"centos"* ]] || [[ "$node_os" == *"oracle"* ]]; then
      cp -f /usr/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
    elif [[ "$node_os" == *"slemicro"* ]]; then
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
      echo -e "$workaround" >/etc/NetworkManager/conf.d/canal.conf
    else
      echo -e "$workaround" >>/etc/NetworkManager/conf.d/canal.conf
    fi
    sudo systemctl reload NetworkManager
  fi
}

install_rke2() {
  url="https://get.rke2.io"
  params="$install_mode=$version INSTALL_RKE2_TYPE=agent"
  
  if [ -n "$channel" ]; then
    params="$params INSTALL_RKE2_CHANNEL=$channel"
  fi

  if [ -n "$install_method" ]; then
    params="$params INSTALL_RKE2_METHOD=$install_method"
  fi

  install_cmd="curl -sfL $url | $params sh -"
  echo "$install_cmd"
  if ! eval "$install_cmd"; then
    echo "Failed to install rke2-agent on joining node ip: $public_ip"
    exit 1
  fi
}

install_dependencies() {
  if [[ "$node_os" = *"rhel"* ]] || [[ "$node_os" = "centos8" ]] || [[ "$node_os" = *"oracle"* ]]; then
    yum install tar iptables -y
  fi
}

enable_service() {
  if ! sudo systemctl enable rke2-agent --now; then
    echo "rke2-agent failed to start on node: $public_ip"

    ## rke2 can sometimes fail to start but some time after it starts successfully.
    sleep 20

    if ! sudo systemctl is-active --quiet rke2-agent; then
      echo "rke2-agent exiting after failed retry to start on node: $public_ip while joining server: $server_ip"
      sudo journalctl -xeu rke2-agent.service --no-pager | grep -i "error\|failed\|fatal"
      exit 1
    else
      echo "rke2-agent started successfully on node: $public_ip"
    fi
  fi
}

install() {
  install_dependencies
  install_rke2
  sleep 10
  cis_setup
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
  fi
  if [[ "${install_or_enable}" == "enable" ]] || [[ "${install_or_enable}" == "both" ]]; then
    echo "Executing ENABLE Block"
    enable_service
  fi
}
main "$@"
