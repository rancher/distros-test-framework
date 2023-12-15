#!/bin/bash
# This script installs the first server, ensuring first server is installed
# and ready before proceeding to install other nodes

echo "$@"
# the following lines are to enable debug mode
set -x
PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

# Scripts args
node_os=${1}
fqdn=${2}
public_ip=${3}
private_ip=${4}
ipv6_ip=${5}
install_mode=${6}
version=${7}
channel=${8}
install_method=${9}
server_flags=${10}
rhel_username=${11}
rhel_password=${12}

create_config() {
  hostname=$(hostname -f)
  mkdir -p /etc/rancher/rke2
  cat << EOF >/etc/rancher/rke2/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${fqdn}
node-name: ${hostname}
EOF
}

update_config() {
  if [ -n "$server_flags" ] && [[ "$server_flags" == *":"* ]]; then
    echo -e "$server_flags" >> /etc/rancher/rke2/config.yaml
    if [[ "$server_flags" != *"cloud-provider-name"* ]]; then
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
  fi
}

cis_setup() {
  if [ -n "$server_flags" ] && [[ "$server_flags" == *"cis"* ]]; then
    if [[ "$node_os" == *"rhel"* ]] || [[ "$node_os" == *"centos"* ]] || [[ "$node_os" == *"oracle"* ]]; then
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

  if [ -n "$install_method" ]; then
    export INSTALL_RKE2_METHOD="$install_method"
  fi

  if [ -z "$channel" ]; then
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

wait_nodes() {
  timeElapsed=0
  while [[ $timeElapsed -lt 1200 ]]
  do
    notready=false
    if [[ ! -f /var/lib/rancher/rke2/server/node-token ]] || [[ ! -f /etc/rancher/rke2/rke2.yaml ]]; then
      notready=true
    fi
    if [[ $notready == false ]]; then
      break
    fi
    sleep 5
    ((timeElapsed+=5))
  done
}

config_files() {
  cat /etc/rancher/rke2/config.yaml > /tmp/joinflags
  cat /var/lib/rancher/rke2/server/node-token > /tmp/nodetoken
  cat /etc/rancher/rke2/rke2.yaml > /tmp/config
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
  config_files
}
main "$@"




