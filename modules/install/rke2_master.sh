#!/bin/bash
# This script installs the first master, ensuring first master is installed
# and ready before proceeding to install other nodes

create_directories() {
mkdir -p /etc/rancher/rke2
}

create_config() {
  local fake_fqdn="${1}"
  hostname=$(hostname -f)

  cat << EOF >>/etc/rancher/rke2/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${fake_fqdn}
node-name: ${hostname}
EOF
}

add_config() {
  local server_flags="${1}"
  local public_ip="${2}"

  if [ -n "$server_flags" ] && [[ "$server_flags" == *":"* ]]
  then
    echo "$server_flags"
    echo -e "$server_flags" >> /etc/rancher/rke2/config.yaml
    if [[ "$server_flags" != *"cloud-provider-name"* ]]
    then
        echo -e "node-external-ip: $public_ip" >> /etc/rancher/rke2/config.yaml
    fi
    cat /etc/rancher/rke2/config.yaml
  else
  echo -e "node-external-ip: $public_ip" >> /etc/rancher/rke2/config.yaml
  fi
}

network_manager() {
  local node_os="${1}"
  local rhel_username="${2}"
  local rhel_password="${3}"

  if [[ "$node_os" = "rhel" ]]
    then
      subscription-manager register --auto-attach --username="$rhel_username" --password="$rhel_password"
      subscription-manager repos --enable=rhel-7-server-extras-rpms
  fi

  if [[ "$node_os" = *"centos"* ]] || [[ "$node_os" = *"rhel"* ]] || [[ "$node_os" = *"oracle"* ]]
  then
  NM_CLOUD_SETUP_SERVICE_ENABLED=$(systemctl status nm-cloud-setup.service | grep -i enabled)
  NM_CLOUD_SETUP_TIMER_ENABLED=$(systemctl status nm-cloud-setup.timer | grep -i enabled)

  if [ "${NM_CLOUD_SETUP_SERVICE_ENABLED}" ]; then
  systemctl disable nm-cloud-setup.service
  fi

  if [ "${NM_CLOUD_SETUP_TIMER_ENABLED}" ]; then
  systemctl disable nm-cloud-setup.timer
  fi

  if [ "$node_os" = "centos8" ] || [ "$node_os" = "rhel8" ] || [ "$node_os" = "oracle8" ]
  then
    yum install tar -y
    yum install iptables -y
    workaround="[keyfile]\nunmanaged-devices=interface-name:cali*;interface-name:tunl*;interface-name:vxlan.calico;interface-name:flannel*"
    if [ ! -e /etc/NetworkManager/conf.d/canal.conf ]; then
      echo -e "$workaround" > /etc/NetworkManager/conf.d/canal.conf
    else
      echo -e "$workaround" >> /etc/NetworkManager/conf.d/canal.conf
    fi
    sudo systemctl reload NetworkManager
  fi
}

install() {
  local install_mode="${1}"
  local rke2_version="${2}"
  local install_method="${3}"
  local rke2_channel="${4}"
  local server_flags="${5}"
  local node_os="${6}"
 
  export "$install_mode"="$rke2_version"
  if [ -n "$install_method" ]
    then
    export INSTALL_RKE2_METHOD="$install_method"
  fi

  if [ -z "$rke2_channel" ]
    then
      curl -sfL https://get.rke2.io |  sh -
  else
    curl -sfL https://get.rke2.io | INSTALL_RKE2_CHANNEL="$rke2_channel" sh -
  fi
  sleep 10
  if [ -n "$server_flags" ] && [[ "$server_flags" == *"cis"* ]]
    then
      if [[ "$node_os" == *"rhel"* ]] || [[ "$node_os" == *"centos"* ]] || [[ "$node_os" == *"oracle"* ]]
        then
            cp -f /usr/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
      else
        cp -f /usr/local/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/60-rke2-cis.conf
    fi
    systemctl restart systemd-sysctl
    useradd -r -c "etcd user" -s /sbin/nologin -M etcd -U
  fi

  sudo systemctl enable rke2-server
  sudo systemctl start rke2-server
}

wait_nodes() {
  export PATH="$PATH:/usr/local/bin"
  timeElapsed=0

  while (( timeElapsed < 300 )); do
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
  timeElapsed=0
  while [[ $timeElapsed -lt 600 ]]
  do
    notready=false
    if [[ ! -f /var/lib/rancher/rke2/server/node-token ]] || [[ ! -f /etc/rancher/rke2/rke2.yaml ]]
      then
        notready=true
    fi
    if [[ $notready == false ]]
      then
        break
    fi
    sleep 5
  ((timeElapsed+=5))
  done
}

config_files() {
  cat /etc/rancher/rke2/config.yaml > /tmp/joinflags
  cat /var/lib/rancher/rke2/server/node-token >/tmp/nodetoken
  cat /etc/rancher/rke2/rke2.yaml >/tmp/config
  export KUBECONFIG=/etc/rancher/rke2/rke2.yaml PATH=$PATH:/var/lib/rancher/rke2/bin:/opt/rke2/bin CRI_CONFIG_FILE=/var/lib/rancher/rke2/agent/etc/crictl.yaml && \
  alias k=kubectl
}

main() {
  create_directories
  create_config "$2" 
  add_config "$6" "$4" 
  network_manager "$1" "$8" "$9" 
  install "$7" "$3" "${10}" "$5" "$6" "$1" 
  wait_ready_nodes
  config_files
}
main "$@"


