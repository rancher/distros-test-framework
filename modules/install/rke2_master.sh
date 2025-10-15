#!/bin/bash
# This script installs the first server, ensuring first server is installed
# and ready before proceeding to install other nodes
# Usage:
# node_os=${1}              # Node OS values. Ex: rhel8, centos8, slemicro
# fqdn=${2}                 # FQDN Value. Value will be added to config.yaml file.
# public_ip=${3}            # Public IP of the master server node. Value will be added to config.yaml file.
# private_ip=${4}           # Private IP of the master server node. Value will be added to config.yaml file.
# ipv6_ip=${5}              # IPv6 of the master node. Value will be added to config.yaml file.
# install_mode=${6}         # Install mode - INSTALL_<K3S|RKE2>_<VERSION|COMMIT>
# version=${7}              # Version or Commit to install
# channel=${8}              # Channel to install from - values can be: testing, latest, stable
# install_method=${9}       # Install Method can be rpm or tar
# datastore_type=${10}      # Datastore type: etcd or external
# datastore_endpoint=${11}  # Datastore endpoint. Value will be added to config.yaml file.
# server_flags=${12}        # Server Flags to add in config.yaml
# rhel_username=${13}       # rhel username
# rhel_password=${14}       # rhel password
# install_or_enable=${15}   # Values can be install, enable or both. In case of slemicro for node_os value, the first time this script is called with 'install'.
                            # After a node reboot, the second time the script is recalled with 'enable' which enables services.
                            # For all other node_os values, this value will be 'both' and this script will be called only once.
# set -x                    # Use for debugging script. Use 'set +x' to turn off debugging at a later stage, if needed.

echo "$@"

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
datastore_type=${10}
datastore_endpoint=${11}
server_flags=${12}
rhel_username=${13}
rhel_password=${14}
install_or_enable=${15}

create_config() {
  hostname=$(hostname -f)
  mkdir -p /etc/rancher/rke2
  cat <<EOF >/etc/rancher/rke2/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${fqdn}
node-name: ${hostname}
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
    else
      echo -e "node-external-ip: $public_ip" >>/etc/rancher/rke2/config.yaml
      echo -e "node-ip: $private_ip" >>/etc/rancher/rke2/config.yaml
    fi
  fi

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
  if [[ -n "$server_flags" && "$server_flags" == *"cis"* ]]; then
    if [[ "$node_os" == *"rhel"* || "$node_os" == *"centos"* || "$node_os" == *"oracle"* ]]; then
      if [[ -f /usr/share/rke2/rke2-cis-sysctl.conf ]]; then
        cp -f /usr/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
      else
        cp -f /usr/local/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
      fi
    elif [[ "$node_os" == *"slemicro"* ]]; then
      echo "Setting up CIS for slemicro"
      groupadd --system etcd && useradd -s /sbin/nologin --system -g etcd etcd
      cat <<EOF >> ~/60-rke2-cis.conf
on_oovm.panic_on_oom=0
vm.overcommit_memory=1
kernel.panic=10
kernel.panic_ps=1
kernel.panic_on_oops=1
EOF
      cp ~/60-rke2-cis.conf /etc/sysctl.d/
      cat /etc/sysctl.d/60-rke2-cis.conf
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
  if [[ "$node_os" == *"sles"* ]]; then
     echo "Checking for package manager locks if so, removing them."
     pkill -f zypper 2>/dev/null || true
     rm -f /var/run/zypp.pid 2>/dev/null || true
     sleep 2
  fi

  url="https://get.rke2.io"
  params="$install_mode=$version"
  
  if [ -n "$channel" ]; then
    params="$params INSTALL_RKE2_CHANNEL=$channel"
  fi

  if [ -n "$install_method" ]; then
    params="$params INSTALL_RKE2_METHOD=$install_method"
  fi

  install_cmd="curl -sfL $url | $params sh -"
  echo "${install_cmd}"
  if ! eval "$install_cmd"; then
    echo "Failed to install rke2-server on node ip: $public_ip"
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
    echo "rke2-server failed to start on node: $public_ip, Waiting for 20s to check if it starts"

    ## rke2 can sometimes fail to start but some time after it starts successfully.
    sleep 20

    if ! sudo systemctl is-active --quiet rke2-server; then
      echo "rke2-server exiting after failed to start on node: $public_ip"
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

wait_nodes() {
  timeElapsed=0
  while [[ $timeElapsed -lt 1200 ]]; do
    notready=false
    if [[ ! -f /var/lib/rancher/rke2/server/node-token ]] || [[ ! -f /etc/rancher/rke2/rke2.yaml ]]; then
      notready=true
    fi
    if [[ $notready == false ]]; then
      break
    fi
    sleep 5
    ((timeElapsed += 5))
  done
}

config_files() {
  cat /etc/rancher/rke2/config.yaml >/tmp/joinflags
  cat /var/lib/rancher/rke2/server/node-token >/tmp/nodetoken
  cat /etc/rancher/rke2/rke2.yaml >/tmp/config
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
  fi
  if [[ "${install_or_enable}" == "enable" ]] || [[ "${install_or_enable}" == "both" ]]; then
    echo "Executing ENABLE Block"
    enable_service
    config_files
    wait_nodes
  fi
}
main "$@"
