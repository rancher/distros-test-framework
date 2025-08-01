#!/bin/bash
# Usage:
# node_os=${1}              # Node OS values. Ex: rhel8, centos8, slemicro
# fqdn=${2}                 # FQDN Value. Value will be added to config.yaml file.
# public_ip=${3}            # Public IP of the master server node. Value will be added to config.yaml file.
# private_ip=${4}           # Private IP of the master server node. Value will be added to config.yaml file.
# ipv6_ip=${5}              # IPv6 IP of the master server node. Value will be added to config.yaml file.
# install_mode=${6}         # Install mode - INSTALL_<K3S|RKE2>_<VERSION|COMMIT>
# version=${7}              # Version or Commit to install
# channel=${8}              # Channel values can be testing, latest or stable.
# etcd_only_node=${9}       # Count of etcd only nodes
# datastore_type=${10}      # Datastore type: etcd or external
# datastore_endpoint=${11}  # Datastore endpoint. Value will be added to config.yaml file.
# server_flags=${12}        # Server Flags to use in config.yaml
# rhel_username=${13}       # rhel username
# rhel_password=${14}       # rhel password
# install_or_enable=${15}   # Values can be install, enable or both. In case of slemicro for node_os value, the first time this script is called with 'install'.
                            # After a node reboot, the second time the script is recalled with 'enable' which enables services.
                            # For all other node_os values, this value will be 'both' and this script will be called only once.
# set -x                    # Use for debugging script. Use 'set +x' to turn off debugging at a later stage, if needed.

PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

# Script args
node_os=${1}
fqdn=${2}
public_ip=${3}
private_ip=${4}
ipv6_ip=${5}
install_mode=${6}
version=${7}
channel=${8}
etcd_only_node=${9}
datastore_type=${10}
datastore_endpoint=${11}
server_flags=${12}
rhel_username=${13}
rhel_password=${14}
install_or_enable=${15}

create_config() {
  hostname=$(hostname -f)
  mkdir -p /etc/rancher/k3s
  cat <<EOF >>/etc/rancher/k3s/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${fqdn}
node-name: ${hostname}
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
    elif [ -n "$ipv6_ip" ] && [ -z "$public_ip" ]; then
      echo -e "node-external-ip: $ipv6_ip" >>/etc/rancher/k3s/config.yaml
    else
      echo -e "node-external-ip: $public_ip" >>/etc/rancher/k3s/config.yaml
      echo -e "node-ip: $private_ip" >>/etc/rancher/k3s/config.yaml
    fi
  fi

  if [ "$datastore_type" = "external" ]; then
    echo -e "datastore-endpoint: $datastore_endpoint" >>/etc/rancher/k3s/config.yaml
  elif [ "$datastore_type" = "etcd" ]; then
    echo -e "cluster-init: true" >>/etc/rancher/k3s/config.yaml
  fi
  cat /etc/rancher/k3s/config.yaml
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
    sleep 5
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
  fi
  if [[ "$node_os" = *"sles"* ]] || [[ "$node_os" = "slemicro" ]]; then
    if [ -n "$ipv6_ip" ]; then
      echo "Configuring sysctl for ipv6"
      sysctl -w net.ipv6.conf.all.accept_ra=2
    fi
  fi
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
    echo "k3s-server is running on node ip $public_ip"
  else
    echo "k3s-server failed to start on node: $public_ip"
    sudo journalctl -xeu k3s.service | grep -i "error\|failed\|fatal"
    exit 1
  fi
}

install() {
  install_k3s
  sleep 10
}

wait_nodes() {
  export PATH="$PATH:/usr/local/bin"
  timeElapsed=0

  while ((timeElapsed < 1200)); do
    if kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get nodes >/dev/null 2>&1; then
      return
    fi
    sleep 5
    ((timeElapsed += 5))
  done

  echo "Timed out while waiting for nodes."
  exit 1
}

wait_ready_nodes() {
  IFS=$'\n'
  timeElapsed=0
  sleep 10

  while ((timeElapsed < 800)); do
    not_ready=false
    for rec in $(kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get nodes); do
      if [[ "$rec" == *"NotReady"* ]]; then
        not_ready=true
        break
      fi
    done
    if [[ $not_ready == false ]]; then
      return
    fi
    sleep 20
    ((timeElapsed += 20))
  done

  echo "Timed out while waiting for ready nodes."
  exit 1
}

wait_pods() {
  IFS=$'\n'
  timeElapsed=0

  while ((timeElapsed < 420)); do
    helmPodsNR=false
    systemPodsNR=false

    for rec in $(kubectl --kubeconfig=/etc/rancher/k3s/k3s.yaml get pods -A --no-headers); do
      if [[ "$rec" == *"helm-install"* ]] && [[ "$rec" != *"Completed"* ]]; then
        helmPodsNR=true
      elif [[ "$rec" != *"helm-install"* ]] && [[ "$rec" != *"Running"* ]]; then
        systemPodsNR=true
      fi
    done

    if [[ $systemPodsNR == false ]] && [[ $helmPodsNR == false ]]; then
      return
    fi

    sleep 20
    ((timeElapsed += 20))
  done

  echo "Timed out while waiting for pods."
  exit 1
}

config_files() {
  cat /etc/rancher/k3s/config.yaml >/tmp/joinflags
  cat /var/lib/rancher/k3s/server/node-token >/tmp/nodetoken
  cat /etc/rancher/k3s/k3s.yaml >/tmp/config
  export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
  sudo chmod 644 /etc/rancher/k3s/k3s.yaml
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
    if [[ "$etcd_only_node" -eq 0 ]]; then
      # If etcd only node count is 0, then wait for nodes/pods to come up.
      # etcd only node needs api server to come up fully, which is in control plane node.
      # and hence we cannot wait for node/pod status in this case.
      wait_nodes
      wait_ready_nodes
      wait_pods
    else
      # add sleep to make sure install finished and the node token file is present on the node for a copy
      sleep 30
    fi
    config_files
  fi
}
main "$@"
