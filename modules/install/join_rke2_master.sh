#!/bin/bash
# This script is used to join one or more nodes as servers to the first server
# Usage:
# node_os=${1}              # Node OS values. Ex: rhel8, centos8, slemicro
# fqdn=${2}                 # FQDN. Value will be added to config.yaml file.
# server_ip=${3}            # Master Server IP to join to. Value will be added to config.yaml file.
# token=${4}                # Node Token. Value will be added to config.yaml file.
# public_ip=${5}            # Public IP of the joining server node. Value will be added to config.yaml file.
# private_ip=${6}           # Private IP of the joining server node. Value will be added to config.yaml file.
# ipv6_ip=${7}              # IPv6 IP of the joining server node. Value will be added to config.yaml file.
# install_mode=${8}         # Install mode - INSTALL_<K3S|RKE2>_<VERSION|COMMIT>
# version=${9}              # Version or Commit to Install
# channel=${10}             # Channel to install from - testing latest or stable
# install_method=${11}      # Method of install - rpm or tar
# datastore_type=${12}      # Datastore type - etcd or external
# datastore_endpoint=${13}  # Datastore Endpoint. Value will be added to config.yaml file.
# server_flags=${14}        # Server Flags to add in config.yaml
# rhel_username=${15}       # rhel username
# rhel_password=${16}       # rhel password
# install_or_enable=${17}   # Values can be install, enable or both. In case of slemicro for node_os value, the first time this script is called with 'install'.
                            # After a node reboot, the second time the script is recalled with 'enable' which enables services.
                            # For all other node_os values, this value will be 'both' and this script will be called only once.
# set -x                    # Use for debugging script. Use 'set +x' to turn off debugging at a later stage, if needed.

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
  if [[ "$node_os" = *"sles"* ]] || [[ "$node_os" = "slemicro" ]]; then
    if [ -n "$ipv6_ip" ]; then
      echo "Configuring sysctl for ipv6"
      echo "net.ipv6.conf.all.accept_ra=2" > /etc/sysctl.d/99-ipv6.conf
      sysctl -p /etc/sysctl.d/99-ipv6.conf
    fi
  fi
}

cis_setup() {
  [[ -z "$server_flags" || "$server_flags" != *"cis"* ]] && return 0

  cis_sysctl_file="/etc/sysctl.d/60-rke2-cis.conf"

  ensure_etcd_user() {
    getent group etcd >/dev/null 2>&1 || groupadd --system etcd
    id -u etcd >/dev/null 2>&1 || useradd -r -s /sbin/nologin -M -g etcd etcd
  }

  case "$node_os" in
    *slemicro*)
      echo "Setting up CIS for SLE Micro"
      cat <<EOF > "$cis_sysctl_file"
vm.overcommit_memory=1
kernel.panic=10
kernel.panic_on_oops=1
EOF
      ;;

    rhel*|centos*|oracle*|sles*|suse*|ubuntu*)
      echo "Applying CIS sysctl file for $node_os"
      src_paths=(
        "/usr/share/rke2/rke2-cis-sysctl.conf"
        "/usr/local/share/rke2/rke2-cis-sysctl.conf"
        "/opt/rke2/share/rke2/rke2-cis-sysctl.conf"
      )

      for path in "${src_paths[@]}"; do
        [[ -f "$path" ]] && { cp -f "$path" "$cis_sysctl_file"; break; }
      done

      [[ ! -f "$cis_sysctl_file" ]] && {
        echo "ERROR: rke2-cis-sysctl.conf not found" >&2
        return 1
      }
      ;;

    *)
      echo "ERROR: CIS mode not supported for OS '$node_os'" >&2
      return 1
      ;;
  esac

  if [[ -n "$server_flags" ]] && [[ "$server_flags" == *"etcd"* ]]; then
      ensure_etcd_user
  fi

  systemctl restart systemd-sysctl
}

install_rke2() {
  if [[ "$node_os" == *"sles"* ]] || [[ "$node_os" == *"slemicro"* ]]; then
     echo "Checking for package manager locks if so, removing them."
     pkill -f zypper 2>/dev/null || true
     rm -f /var/run/zypp.pid 2>/dev/null || true
     sleep 2
  fi

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
    echo "rke2-server failed to start on node: $public_ip, Waiting up to 5 minutes..."

    max_wait=300 
    elapsed=0
    
    while [ $elapsed -lt $max_wait ]; do
      if sudo systemctl is-active --quiet rke2-server; then
        echo "rke2-server started successfully on node: $public_ip after ${elapsed}s"
        return 0
      fi
      sleep 5
      elapsed=$((elapsed + 5))
    done
    
    echo "rke2-server exiting after failed to start on node: $public_ip"
    sudo journalctl -xeu rke2-server.service --no-pager | grep -i "failed\|fatal"
    exit 1
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
