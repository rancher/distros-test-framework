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
    bgpControlPlane:
      enabled: true
      announce:
        podCIDR: true
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
