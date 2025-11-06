#!/bin/bash
# Usage:
# node_os=${1}              # Node OS values. Ex: rhel8, centos8, slemicro
# fqdn=${2}                 # FQDN. Value will be added to config.yaml file.
# server_ip=${3}            # Master Server IP to join to. Value will be added to config.yaml file.
# token=${4}                # Node Token. Value will be added to config.yaml file.
# public_ip=${5}            # Public IP of the joining server node. Value will be added to config.yaml file.
# private_ip=${6}           # Privaate IP of the joining server node. Value will be added to config.yaml file.
# ipv6_ip=${7}              # IPV6 IP of the joining server node. Value will be added to config.yaml file.
# install_mode=${8}         # Install mode - INSTALL_<K3S|RKE2>_<VERSION|COMMIT>
# version=${9}              # Version or Commit to install
# channel=${10}             # Channel to install from - values can be testing, latest or stable
# datastore_type=${11}      # Datastore type can be etcd or external
# datastore_endpoint=${12}  # Datastore endpoint. Value will be added to config.yaml file.
# server_flags=${13}        # Server Flags to add in config.yaml
# rhel_username=${14}       # rhel username
# rhel_password=${15}       # rhel Password
# install_or_enable=${16}   # Values can be install, enable or both. In case of slemicro for node_os value, the first time this script is called with 'install'.
                            # After a node reboot, the second time the script is recalled with 'enable' which enables services.
                            # For all other node_os values, this value will be 'both' and this script will be called only once.
# set -x                    # Use for debugging script. Use 'set +x' to turn off debugging at a later stage, if needed.

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
install_or_enable=${16}

create_config() {
  hostname=$(hostname -f)
  sudo mkdir -p /etc/rancher/k3s
  cat <<EOF >>/etc/rancher/k3s/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${fqdn}
token: "${token}"
EOF
}

update_config() {
  if [ -n "$server_flags" ] && [[ "$server_flags" == *":"* ]]; then
    echo -e "$server_flags" >>/etc/rancher/k3s/config.yaml
  fi

  if [[ "$server_flags" != *"cloud-provider-name"* ]] || [[ -z "$server_flags" ]]; then
    if [ -n "$ipv6_ip" ] && [ -n "$public_ip" ] && [ -n "$private_ip" ]; then
      echo -e "node-external-ip: $public_ip,$ipv6_ip" >>/etc/rancher/k3s/config.yaml
      echo -e "node-ip: $private_ip,$ipv6_ip" >>/etc/rancher/k3s/config.yaml
    elif [ -n "$ipv6_ip" ]; then
      echo -e "node-external-ip: $ipv6_ip" >>/etc/rancher/k3s/config.yaml
      server_ip="[$server_ip]"
      hostname="$hostname-srv$RANDOM"
      echo -e "disable-network-policy: true" >>/etc/rancher/k3s/config.yaml
      echo -e "flannel-ipv6-masq: true" >>/etc/rancher/k3s/config.yaml
    else
      echo -e "node-external-ip: $public_ip" >>/etc/rancher/k3s/config.yaml
      echo -e "node-ip: $private_ip" >>/etc/rancher/k3s/config.yaml
    fi
  fi
  echo -e server: https://"${server_ip}":6443 >>/etc/rancher/k3s/config.yaml
  echo -e node-name: "${hostname}" >>/etc/rancher/k3s/config.yaml

  if [ "$datastore_type" = "external" ]; then
    echo -e "datastore-endpoint: $datastore_endpoint" >>/etc/rancher/k3s/config.yaml
  fi
  cat /etc/rancher/k3s/config.yaml
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
  fi
  if [[ "$node_os" = *"sles"* ]] || [[ "$node_os" = "slemicro" ]]; then
    if [ -n "$ipv6_ip" ]; then
      echo "Configuring sysctl for ipv6"
      echo "net.ipv6.conf.all.accept_ra=2" > ~/99-ipv6.conf
      cp ~/99-ipv6.conf /etc/sysctl.d/99-ipv6.conf
      sysctl -p /etc/sysctl.d/99-ipv6.conf
      systemctl restart systemd-sysctl
    fi
  fi
}

policy_files() {
  if [[ -n "$server_flags" ]] && [[ "$server_flags" == *"protect-kernel-defaults"* ]]; then
    sudo mkdir -p -m 700 /var/lib/rancher/k3s/server/logs
    sudo mkdir -p -m 700 /var/lib/rancher/k3s/server/manifests
    cat /tmp/cis_master_config.yaml >>/etc/rancher/k3s/config.yaml
    printf "%s\n" "vm.panic_on_oom=0" "vm.overcommit_memory=1" "kernel.panic=10" "kernel.panic_on_oops=1" "kernel.keys.root_maxbytes=25000000" >>/etc/sysctl.d/90-kubelet.conf
    sysctl -p /etc/sysctl.d/90-kubelet.conf
    systemctl restart systemd-sysctl
    cat /tmp/policy.yaml >/var/lib/rancher/k3s/server/manifests/policy.yaml
    cat /tmp/admission-config.yaml >/var/lib/rancher/k3s/server/admission-config.yaml
    cat /tmp/audit.yaml >/var/lib/rancher/k3s/server/audit.yaml
    cat /tmp/ingresspolicy.yaml >/var/lib/rancher/k3s/server/manifests/ingresspolicy.yaml
  fi
  sleep 5
}

install_k3s() {
  if [[ "$node_os" == *"sles"* ]] || [[ "$node_os" == *"slemicro"* ]]; then
      echo "Checking for package manager locks if so, removing them."
      pkill -f zypper 2>/dev/null || true
      rm -f /var/run/zypp.pid 2>/dev/null || true
      sleep 2
  fi

  url="https://get.k3s.io"
  params="$install_mode=$version"

  if [[ -n "$channel" ]]; then
    params="$params INSTALL_K3S_CHANNEL=$channel"
  fi

  if [[ "$install_or_enable" == "install" ]]; then
    params="$params INSTALL_K3S_SKIP_ENABLE=true"
  fi

  install_cmd="curl -sfL $url | $params sh -"
  echo "$install_cmd"
  if ! eval "$install_cmd"; then
    echo "Failed to install k3s-server on node: $public_ip"
    exit 1
  fi
}

enable_service() {
  if ! sudo systemctl enable k3s --now; then
    echo "k3s server to start on node: $public_ip, Waiting for 10s for retry..."
    sleep 10

    if ! sudo systemctl is-active --quiet k3s; then
      echo "k3s server exiting after failed retry to start on node: $public_ip"
      sudo journalctl -xeu k3s.service --no-pager | grep -i "failed\|fatal"
      exit 1
    else
      echo "k3s server started successfully on node: $public_ip"
    fi
  fi
}

check_service() {
  if systemctl is-active --quiet k3s; then
    echo "k3s-server is running on node: $public_ip"
  else
    echo "k3s-server failed to start on node: $public_ip while joining server: $server_ip"
    sudo journalctl -xeu k3s.service | grep -i "error\|failed\|fatal"
    exit 1
  fi
}

install() {
  install_k3s
  sleep 10
}

main() {
  echo "Install or enable or both? $install_or_enable"
  if [[ "${install_or_enable}" == "install" ]] || [[ "${install_or_enable}" == "both" ]]; then
    create_config
    update_config
    policy_files
    subscription_manager
    disable_cloud_setup
    install
  fi
  if [[ "${install_or_enable}" == "enable" ]]; then
    enable_service
    sleep 10
  fi
  if [[ "${install_or_enable}" == "enable" ]] || [[ "${install_or_enable}" == "both" ]]; then
    check_service
  fi
}
main "$@"
