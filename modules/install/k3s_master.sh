#!/bin/bash

# the following lines are to enable debug mode
set -x
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

create_config() {
  hostname=$(hostname -f)
  mkdir -p /etc/rancher/k3s
  cat <<EOF >>/etc/rancher/k3s/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${fqdn}
cluster-init: true
node-name: $hostname
EOF
}

update_config() {
  if [ -n "$server_flags" ] && [[ "$server_flags" == *":"* ]]; then
    echo -e "$server_flags" >> /etc/rancher/k3s/config.yaml
  fi

  if [[ "$server_flags" != *"cloud-provider-name"* ]]; then
    if [ -n "$ipv6_ip" ] && [ -n "$public_ip" ] && [ -n "$private_ip" ]; then
      echo -e "node-external-ip: $public_ip,$ipv6_ip" >> /etc/rancher/k3s/config.yaml
      echo -e "node-ip: $private_ip,$ipv6_ip" >> /etc/rancher/k3s/config.yaml
    elif [ -n "$ipv6_ip" ]; then
      echo -e "node-external-ip: $ipv6_ip" >> /etc/rancher/k3s/config.yaml
      echo -e "node-ip: $ipv6_ip" >> /etc/rancher/k3s/config.yaml
    else
      echo -e "node-external-ip: $public_ip" >> /etc/rancher/k3s/config.yaml
      echo -e "node-ip: $private_ip" >> /etc/rancher/k3s/config.yaml
    fi
  fi
  cat /etc/rancher/k3s/config.yaml
}

policy_files() {
  if [[ -n "$server_flags" ]] && [[ "$server_flags"  == *"protect-kernel-defaults"* ]]; then
    sudo mkdir -p -m 700 /var/lib/rancher/k3s/server/logs
    mkdir -p /var/lib/rancher/k3s/server/manifests
    cat /tmp/cis_master_config.yaml >> /etc/rancher/k3s/config.yaml
    printf "%s\n" "vm.panic_on_oom=0" "vm.overcommit_memory=1" "kernel.panic=10" "kernel.panic_on_oops=1" "kernel.keys.root_maxbytes=25000000" >> /etc/sysctl.d/90-kubelet.conf
    sysctl -p /etc/sysctl.d/90-kubelet.conf
    systemctl restart systemd-sysctl
    cat /tmp/policy.yaml > /var/lib/rancher/k3s/server/manifests/policy.yaml
    cat /tmp/audit.yaml > /var/lib/rancher/k3s/server/audit.yaml
    cat /tmp/cluster-level-pss.yaml > /var/lib/rancher/k3s/server/cluster-level-pss.yaml
    cat /tmp/ingresspolicy.yaml > /var/lib/rancher/k3s/server/manifests/ingresspolicy.yaml
    sleep 20
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
}

etcd_download() {
    local etcd_version="v3.5.0"

    sudo curl -L https://github.com/etcd-io/etcd/releases/download/${etcd_version}/etcd-${etcd_version}-linux-amd64.tar.gz -o etcd-${etcd_version}-linux-amd64.tar.gz
    sudo tar xzvf etcd-${etcd_version}-linux-amd64.tar.gz -C /usr/local/bin --strip-components=1 etcd-${etcd_version}-linux-amd64/etcdctl
}

install_etcdctl() {
  if [[ "$node_os" == *"rhel"* ]] || [[ "$node_os" == *"centos"* ]] || [[ "$node_os" == *"oracle"* ]]; then
      yum update -y > /dev/null 2>&1
      sudo dnf install -y tar
      etcd_download
    elif [[ "$node_os" == *"ubuntu"* ]]; then
      apt-get update -y > /dev/null 2>&1
      etcd_download
    else
      zypper update -y > /dev/null 2>&1
      etcd_download
  fi
}

install() {
  export "$install_mode"="$version"

  if [ "$datastore_type" = "etcd" ]; then
    if [[ -n "$channel" ]]; then
      curl -sfL https://get.k3s.io | INSTALL_K3S_CHANNEL=$channel INSTALL_K3S_TYPE='server' sh -s - server
      install_etcdctl
    else
      curl -sfL https://get.k3s.io | INSTALL_K3S_TYPE='server' sh -s - server
      install_etcdctl
    fi
  elif  [[ "$datastore_type" = "external" ]]; then
    if [[ -n "$channel" ]]; then
      curl -sfL https://get.k3s.io | INSTALL_K3S_CHANNEL=$channel sh -s - server --datastore-endpoint="$datastore_endpoint"
    else
      curl -sfL https://get.k3s.io | sh -s - server  --datastore-endpoint="$datastore_endpoint"
    fi
  fi
}

wait_nodes() {
  export PATH="$PATH:/usr/local/bin"
  timeElapsed=0

  while (( timeElapsed < 1200 )); do
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

  while (( timeElapsed < 800 )); do
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

  while (( timeElapsed < 420 )); do
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
  cat /etc/rancher/k3s/config.yaml > /tmp/joinflags
  cat /var/lib/rancher/k3s/server/node-token > /tmp/nodetoken
  cat /etc/rancher/k3s/k3s.yaml > /tmp/config
  export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
  sudo chmod 644 /etc/rancher/k3s/k3s.yaml
}

main() {
  create_config
  update_config
  policy_files
  subscription_manager
  disable_cloud_setup
  install
  if [ "$etcd_only_node" -eq 0 ]; then
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
}
main "$@"
