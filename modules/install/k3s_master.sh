#!/bin/bash

## Uncomment the following lines to enable debug mode
# set -x
# PS4='+(${LINENO}): '
# set -e
# trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

create_directories() {
  mkdir -p /etc/rancher/k3s
  sudo mkdir -p -m 700 /var/lib/rancher/k3s/server/logs
  mkdir -p /var/lib/rancher/k3s/server/manifests
}

create_config() {
  local fake_fqdn="${1}"
  local node_external_ip="${2}"

  cat <<EOF >>/etc/rancher/k3s/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${fake_fqdn}
node-external-ip: "${node_external_ip}"
cluster-init: true
EOF
}

add_config() {
  local server_flags="${1}"

  if [[ -n "$server_flags" ]] && [[ "$server_flags" == *":"* ]]
    then
      echo -e "$server_flags" >> /etc/rancher/k3s/config.yaml
      cat /etc/rancher/k3s/config.yaml
  fi
}

policy_files() {
  local server_flags="${1}"
  local version="${2}"

  if [[ -n "$server_flags"  ]] && [[ "$server_flags"  == *"protect-kernel-defaults"* ]]
    then
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
   local node_os="${1}"
   local username="${2}"
   local password="${3}"

  if [ "$node_os" = "rhel" ]
    then
      subscription-manager register --auto-attach --username="$username" --password="$password"
      subscription-manager repos --enable=rhel-7-server-extras-rpms
  fi
}

disable_cloud_setup() {
   local node_os="${1}"

if  [[ "$node_os" = *"rhel"* ]] || [[ "$node_os" = "centos8" ]]
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

export "${3}"="${4}"

install() {
    local datastore_type="${1}"
    local version="${2}"
    local channel="${3}"
    local datastore_endpoint="${4}"

  if [ "$datastore_type" = "etcd" ]
  then
     if [[ "$version" == *"v1.18"* ]] || [[ "$version" == *"v1.17"* ]]
     then
         curl -sfL https://get.k3s.io | INSTALL_K3S_TYPE='server' sh -s - server
     else
         if [[ -n "$channel" ]]
         then
             curl -sfL https://get.k3s.io | INSTALL_K3S_CHANNEL=$channel INSTALL_K3S_TYPE='server' sh -s - server
         else
             curl -sfL https://get.k3s.io | INSTALL_K3S_TYPE='server' sh -s - server
         fi
     fi
  elif  [[ "$datastore_type" = "external" ]]
   then
    if [[ "$version" == *"v1.18"* ]] || [[ "$version" == *"v1.17"* ]]
    then
        curl -sfL https://get.k3s.io | sh -s - server --datastore-endpoint="$datastore_endpoint"
    else
        if [[ -n "$channel" ]]
        then
            curl -sfL https://get.k3s.io | INSTALL_K3S_CHANNEL=$channel sh -s - server --datastore-endpoint="$datastore_endpoint"
        else
            curl -sfL https://get.k3s.io | sh -s - server  --datastore-endpoint="$datastore_endpoint"
        fi
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
  cat /etc/rancher/k3s/config.yaml> /tmp/joinflags
  cat /var/lib/rancher/k3s/server/node-token >/tmp/nodetoken
  cat /etc/rancher/k3s/k3s.yaml >/tmp/config
  export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
  sudo chmod 644 /etc/rancher/k3s/k3s.yaml
}

main() {
  create_directories
  create_config "$2" "$6"
  add_config "$8"
  policy_files "$8" "$4"
  rhel "$1" "$9" "${10}"
  disable_cloud_setup "$1"
  install "$5" "$4" "${11}" "$7"
  wait_nodes
  wait_ready_nodes
  wait_pods
  config_files
}
main "$@"