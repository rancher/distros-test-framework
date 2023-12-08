#!/bin/bash

## Uncomment the following lines to enable debug mode
#set -x
# PS4='+(${LINENO}): '
set -x
echo "$@"

node_os=${1}
fqdn=${2}
public_ip=${3}
private_ip=${4}
ipv6_ip=${5}
install_mode=${6}
version=${7}
channel=${8}
datastore_type=${9}
datastore_endpoint=${10}
server_flags=${11}
rhel_username=${12}
rhel_password=${13}

create_directories() {
  sudo mkdir -p -m 700 /var/lib/rancher/k3s/server/logs
  mkdir -p /var/lib/rancher/k3s/server/manifests
}

create_config() {
  hostname=$(hostname -f)
  mkdir -p /etc/rancher/k3s
  cat <<EOF >>/etc/rancher/k3s/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - $fqdn
cluster-init: true
node-name: ${hostname}
EOF
}

add_config() {
  if [ -n "$server_flags" ] && [[ "$server_flags" == *":"* ]]
  then
    echo -e "$server_flags" >> /etc/rancher/k3s/config.yaml
  fi

  if [[ "$server_flags" != *"cloud-provider-name"* ]]
  then
    if [ -n "$ipv6_ip" ] && [ -n "$public_ip" ] && [ -n "$private_ip" ]
    then
      echo -e "node-external-ip: $public_ip,$ipv6_ip" >> /etc/rancher/k3s/config.yaml
      echo -e "node-ip: $private_ip,$ipv6_ip" >> /etc/rancher/k3s/config.yaml
    elif [ -n "$ipv6_ip" ]
    then
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
  if [[ -n "$server_flags"  ]] && [[ "$server_flags"  == *"protect-kernel-defaults"* ]]
  then
    create_directories
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

rhel() {
  if [ "$node_os" = "rhel" ]
  then
    subscription-manager register --auto-attach --username="$rhel_username" --password="$rhel_password"
    subscription-manager repos --enable=rhel-7-server-extras-rpms
  fi
}

disable_cloud_setup() {
  if  [[ "$node_os" = *"rhel"* ]] || [[ "$node_os" = *"centos"* ]]
  then
    NM_CLOUD_SETUP_SERVICE_ENABLED=$(systemctl status nm-cloud-setup.service | grep -i enabled)
    NM_CLOUD_SETUP_TIMER_ENABLED=$(systemctl status nm-cloud-setup.timer | grep -i enabled)

    if [ "${NM_CLOUD_SETUP_SERVICE_ENABLED}" ]; then
      systemctl disable nm-cloud-setup.service
    fi

    if [ "${NM_CLOUD_SETUP_TIMER_ENABLED}" ]; then
      systemctl disable nm-cloud-setup.timer
    fi
  fi
}

export "$install_mode"="$version"

install() {
  if [ "$datastore_type" = "etcd" ]
  then
    echo "Datastore Type is $datastore_type"
    if [[ -n "$channel" ]]
    echo "Channel is $channel"
    then
      curl -sfL https://get.k3s.io | INSTALL_K3S_CHANNEL=$channel INSTALL_K3S_TYPE='server' sh -s - server
    else
      curl -sfL https://get.k3s.io | INSTALL_K3S_TYPE='server' sh -s - server
    fi
  else
    echo "Datastore Type is $datastore_type"
    if [[ -n "$channel" ]]
    echo "Channel is $channel"
    then
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
    if kubectl get nodes >/dev/null 2>&1; then
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
    for rec in $(kubectl get nodes); do
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

    for rec in $(kubectl get pods -A --no-headers); do
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
  cat <<EOF >> .bashrc
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml PATH=$PATH:/var/lib/rancher/k3s/bin:/opt/k3s/bin && \
alias k=kubectl
EOF
source .bashrc
}

main() {
  create_config
  add_config
  policy_files
  rhel
  disable_cloud_setup
  install
  wait_nodes
  wait_ready_nodes
  wait_pods
  config_files
}
main "$@"