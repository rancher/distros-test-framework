#!/bin/bash
# This script is used to join one or more nodes as servers to the first sertver
echo "$@"
# the following lines are to enable debug mode
set -x
PS4='+(${LINENO}): '
set -e
trap 'echo "Error on line $LINENO: $BASH_COMMAND"' ERR

node_os=$1
create_lb=$2
initial_node_ip=$3
token=$4
rke2_version=$5
public_ip=$6
rke2_channel=$7
server_flags=$8
install_mode=$9
rhel_username=${10}
rhel_password=${11}
install_method=${12}

hostname=$(hostname -f)
mkdir -p /etc/rancher/rke2
cat <<EOF >>/etc/rancher/rke2/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${create_lb}
server: https://${initial_node_ip}:9345
token:  "${token}"
node-name: "${hostname}"
EOF

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

if [ "$node_os" = "rhel" ]
then
    subscription-manager register --auto-attach --username="$rhel_username" --password="$rhel_password" || echo "Failed to register or attach subscription."

    subscription-manager repos --enable=rhel-7-server-extras-rpms || echo "Failed to enable repositories."
fi

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
    echo -e "$workaround" > /etc/NetworkManager/conf.d/canal.conf
  else
    echo -e "$workaround" >> /etc/NetworkManager/conf.d/canal.conf
  fi
  sudo systemctl reload NetworkManager
fi

export "$install_mode"="$rke2_version"
if [ -n "$install_method" ]
then
  export INSTALL_RKE2_METHOD="$install_method"
fi

if [ "$rke2_channel" != "null" ]
then
    curl -sfL https://get.rke2.io | INSTALL_RKE2_CHANNEL="$rke2_channel" sh -
else
    curl -sfL https://get.rke2.io | sh -
fi
sleep 10
yum install tar iptables -y

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
sudo systemctl start --no-block rke2-server
cat <<EOF >> .bashrc
export KUBECONFIG=/etc/rancher/rke2/rke2.yaml PATH=$PATH:/var/lib/rancher/rke2/bin:/opt/rke2/bin CRI_CONFIG_FILE=/var/lib/rancher/rke2/agent/etc/crictl.yaml && \
alias k=kubectl
EOF
source .bashrc