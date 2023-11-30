#!/bin/bash

## Uncomment the following lines to enable debug mode
#set -x
# PS4='+(${LINENO}): '
#set -e
#trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR


create_directories() {
  sudo mkdir -p /etc/rancher/k3s
  sudo mkdir -p -m 700 /var/lib/rancher/k3s/server/logs
  sudo mkdir -p /var/lib/rancher/k3s/server/manifests
  echo "alias k=kubectl" >> ~/.bashrc
  source ~/.bashrc
}

create_config() {
  local fake_fqdn="${1}"
  local server_ip="${2}"
  local token="${3}"
  local node_external_ip="${4}"

  cat <<EOF >>/etc/rancher/k3s/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${fake_fqdn}
server: https://${server_ip}:6443
token: "${token}"
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
  alias k=kubectl
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

if [[ "$node_os" == *"rhel"* ]] || [[ "$node_os" == "centos8" ]]
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

policy_files() {
  local server_flags="${1}"
  local version="${2}"

  if [[ -n "$server_flags"  ]] && [[ "$server_flags"  == *"protect-kernel-defaults"* ]]
    then
      cat /tmp/cis_server_config.yaml >> /etc/rancher/k3s/config.yaml
      printf "%s\n" "vm.panic_on_oom=0" "vm.overcommit_memory=1" "kernel.panic=10" "kernel.panic_on_oops=1" "kernel.keys.root_maxbytes=25000000" >> /etc/sysctl.d/90-kubelet.conf
      sysctl -p /etc/sysctl.d/90-kubelet.conf
      systemctl restart systemd-sysctl
      cat /tmp/policy.yaml > /var/lib/rancher/k3s/server/manifests/policy.yaml
      cat /tmp/audit.yaml > /var/lib/rancher/k3s/server/audit.yaml
      cat /tmp/cluster-level-pss.yaml > /var/lib/rancher/k3s/server/cluster-level-pss.yaml
      cat /tmp/ingresspolicy.yaml > /var/lib/rancher/k3s/server/manifests/ingresspolicy.yaml
  fi
  sleep 20
}

export "${3}"="${4}"

install() {
    local datastore_type="${1}"
    local version="${2}"
    local channel="${3}"
    local datastore_endpoint="${4}"

    if [ "$datastore_type" = "etcd" ]; then
        if [[ "$version" == *"v1.18"* ]] || [[ "$version" == *"v1.17"* ]]
         then
            curl -sfL https://get.k3s.io | INSTALL_K3S_TYPE='server' sh -
        else
            if [[ -n "$channel" ]]; then
                curl -sfL https://get.k3s.io | INSTALL_K3S_CHANNEL=$channel INSTALL_K3S_TYPE='server' sh -
            else
                curl -sfL https://get.k3s.io | INSTALL_K3S_TYPE='server' sh -
            fi
        fi
    else
        if [[ "$version" == *"v1.18"* ]] || [[ "$version" == *"v1.17"* ]]
          then
            curl -sfL https://get.k3s.io | INSTALL_K3S_TYPE='server' sh -s - server --datastore-endpoint="$datastore_endpoint"
        else
            if [[ -n "$channel" ]]; then
                curl -sfL https://get.k3s.io | INSTALL_K3S_CHANNEL=$channel INSTALL_K3S_TYPE='server' sh -s - server --datastore-endpoint="$datastore_endpoint"
            else
                curl -sfL https://get.k3s.io | INSTALL_K3S_TYPE='server' sh -s - server  --datastore-endpoint="$datastore_endpoint"
            fi
        fi
    fi
}

main() {
  create_directories
  create_config "$2" "$7" "$8" "$6"
  add_config  "${10}"
  policy_files "${10}" "$4"
  rhel "$1" "${11}" "${12}"
  disable_cloud_setup "$1"
  install "$5" "$4" "${13}" "$9"
}
main "$@"