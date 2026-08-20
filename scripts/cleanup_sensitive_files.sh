#!/bin/sh
set -eu

workspace=${1:-.}
workspace=$(cd "$workspace" && pwd -P)
if [ "$workspace" = "/" ]; then
    echo "Refusing to clean the filesystem root" >&2
    exit 1
fi

rm -f -- \
    "$workspace/.env" \
    "$workspace/.qase.env" \
    "$workspace/config/.env" \
    "$workspace/config/k3s.tfvars" \
    "$workspace/config/rke2.tfvars" \
    "$workspace/config/.ssh/aws_key.pem" \
    "$workspace/infrastructure/qainfra/vars.tfvars"
