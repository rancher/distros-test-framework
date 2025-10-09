#!/bin/bash

set +x

instance_id=${1}
product=${2}
flags=${3}

ipv6_config() {
  echo "Stopping systemd-resolved"
  systemctl stop systemd-resolved.service
  echo "Updating /etc/hosts"
  sed -i -e 's/127.0.0.1/::1/g' -e "s/ip6-loopback/ip6-loopback $instance_id/g" /etc/hosts
  echo "Updating /etc/resolv.conf"
  sed -i 's/127.0.0.53/2a00:1098:2c::1/g' /etc/resolv.conf
  echo "Configuring sysctl for ipv6"
  echo "net.ipv6.conf.all.accept_ra=2" > ~/99-ipv6.conf
  cp ~/99-ipv6.conf /etc/sysctl.d/99-ipv6.conf
  sysctl -p /etc/sysctl.d/99-ipv6.conf
  systemctl restart systemd-sysctl
  sleep 2
}

# Ref: https://github.com/rancher/rke2/issues/8033
cilium_config() {
  echo "Setting helmchartconfig for cilium with ipv6only"
  if [[ "$flags" =~ "cilium" ]]; then
    mkdir -p /var/lib/rancher/rke2/server/manifests
    cat <<EOF >>/var/lib/rancher/rke2/server/manifests/rke2-cilium-ipv6config.yaml
---
apiVersion: helm.cattle.io/v1
kind: HelmChartConfig
metadata:
  name: rke2-cilium
  namespace: kube-system
spec:
  valuesContent: |-
    autoDirectNodeRoutes: true
EOF
  fi
}

main() {
  ipv6_config
  if [[ "$product" == "rke2" ]] && [ -n "$flags" ]; then
    cilium_config
  fi
}

main "$@"
