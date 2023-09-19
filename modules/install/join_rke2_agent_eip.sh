#!/bin/bash
# This script is used to join one or more nodes as agents
set -x
echo "$@"

server_ip=$1
token=$2
public_ip=$3
worker_flags=$4

hostname=$(hostname -f)
mkdir -p /etc/rancher/rke2
cat <<EOF >/etc/rancher/rke2/config.yaml
server: https://${server_ip}:9345
token:  "${token}"
node-name: "${hostname}"
EOF

if [ -n "$worker_flags" ] && [[ "$worker_flags" == *":"* ]]
then
   echo "$worker_flags"
   echo -e "$worker_flags" >> /etc/rancher/rke2/config.yaml
   if [[ "$worker_flags" != *"cloud-provider-name"* ]]
   then
     echo -e "node-external-ip: $public_ip" >> /etc/rancher/rke2/config.yaml
   fi
   cat /etc/rancher/rke2/config.yaml
else
  echo -e "node-external-ip: $public_ip" >> /etc/rancher/rke2/config.yaml
fi

sudo systemctl restart rke2-agent