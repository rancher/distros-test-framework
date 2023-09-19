#!/bin/bash
# This script is used to join one or more nodes as masters to the first master
set -x
echo "$@"

initial_node_ip=$1
token=$2
public_ip=$3
server_flags=$4

hostname=$(hostname -f)
mkdir -p /etc/rancher/rke2
cat <<EOF >/etc/rancher/rke2/config.yaml
write-kubeconfig-mode: "0644"
tls-san:
  - ${initial_node_ip}
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

sudo systemctl restart --no-block rke2-server
