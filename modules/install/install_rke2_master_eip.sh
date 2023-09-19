#!/bin/bash
# This script installs the first master, ensuring first master is installed
# and ready before proceeding to install other nodes
set -x
echo "$@"

create_lb=$1
public_ip=$2
server_flags=$3

hostname=$(hostname -f)
mkdir -p /etc/rancher/rke2
cat << EOF >/etc/rancher/rke2/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${create_lb}
node-name: ${hostname}
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

sudo systemctl restart rke2-server

