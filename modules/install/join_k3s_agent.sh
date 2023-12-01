#!/bin/bash

## Uncomment the following lines to enable debug mode
# set -x
# PS4='+(${LINENO}): '
# set -e
# trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

create_config() {
  mkdir -p /etc/rancher/k3s
  local server_ip="${1}"
  local token="${2}"

  cat <<EOF >>/etc/rancher/k3s/config.yaml
server: https://${1}:6443
token:  "${2}"
EOF
}

add_config() {
  local worker_flags="${1}"

  if [[ -n "$worker_flags" ]] && [[ "$worker_flags" == *":"* ]]
    then
      echo -e "$worker_flags" >> /etc/rancher/k3s/config.yaml
      cat /etc/rancher/k3s/config.yaml
  fi

  if [[ -n "$worker_flags" ]] && [[ "$worker_flags" == *"protect-kernel-defaults"* ]]
    then
       cat /tmp/cis_worker_config.yaml >> /etc/rancher/k3s/config.yaml
       printf "%s\n" "vm.panic_on_oom=0" "vm.overcommit_memory=1" "kernel.panic=10" "kernel.panic_on_oops=1" "kernel.keys.root_maxbytes=25000000" >> /etc/sysctl.d/90-kubelet.conf
       sysctl -p /etc/sysctl.d/90-kubelet.conf
       systemctl restart systemd-sysctl
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

export "${2}"="${3}"

install(){
  local version="${1}"
  local worker_flags="${2}"
  local install_mode="${3}"
  local ip="${4}"
  local server_ip="${5}"
  local token="${6}"
  local channel="${7}"

if [[ "$version" == *"v1.18"* ]] || [[ "$version" == *"v1.17"* ]] && [[ -n "$worker_flags" ]]
  then
    curl -sfL https://get.k3s.io | sh -s - "$install_mode" --node-external-ip="$ip" --server https://"$server_ip":6443 --token "$token"
else
    if [[ -n "$channel"  ]]
    then
      curl -sfL https://get.k3s.io | INSTALL_K3S_CHANNEL=$channel sh -s - agent --node-external-ip="$ip"
    else
      curl -sfL https://get.k3s.io | sh -s - agent --node-external-ip="$ip"
    fi
    sleep 10
fi
}

main() {
  create_config "$4" "$5"
  add_config "$7"
  rhel "$1" "$8" "$9"
  disable_cloud_setup "$1"
  install "$3" "$7" "$2" "$6" "$4" "$5" "${10}"
}
main "$@"