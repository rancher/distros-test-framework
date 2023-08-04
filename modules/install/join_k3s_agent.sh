#!/bin/bash

## Uncomment the following lines to enable debug mode
#set -x
# PS4='+(${LINENO}): '

mkdir -p /etc/rancher/k3s

create_config() {
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
   local os_name="${1}"
   local username="${2}"
   local password="${3}"

  if [ "$os_name" = "rhel" ]
    then
      subscription-manager register --auto-attach --username="$username" --password="$password"
      subscription-manager repos --enable=rhel-7-server-extras-rpms
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
  install "$3" "$7" "$2" "$6" "$4" "$5" "${10}"
}
main "$@"