#!/bin/bash

set +x

instance_id=${1}
product=${2}

ipv6_config() {
  echo "Stopping systemd-resolved"
  systemctl stop systemd-resolved.service
  echo "Updating /etc/hosts"
  sed -i -e 's/127.0.0.1/::1/g' -e "s/ip6-loopback/ip6-loopback $instance_id/g" /etc/hosts
  echo "Updating /etc/resolv.conf"
  sed -i 's/127.0.0.53/2a00:1098:2c::1/g' /etc/resolv.conf
}

main() {
  ipv6_config
}

main "$@"
