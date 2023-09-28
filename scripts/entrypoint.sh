#!/bin/sh

CONFIG_PATH="/go/src/github.com/rancher/distros-test-framework/config/${ENV_PRODUCT}.tfvars"

[ -n "$K3S_VERSION" ] && sed -i "s/k3s_version\s*=\s*.*/k3s_version = \"$K3S_VERSION\"/" $CONFIG_PATH
[ -n "$RKE2_VERSION" ] && sed -i "s/rke2_version\s*=\s*.*/rke2_version = \"$RKE2_VERSION\"/" $CONFIG_PATH
[ -n "$INSTALL_MODE" ] && sed -i "s/install_mode\s*=\s*.*/install_mode = \"$INSTALL_MODE\"/" $CONFIG_PATH
[ -n "$NO_OF_SERVER_NODES" ] && sed -i "s/no_of_server_nodes = .*/no_of_server_nodes = $NO_OF_SERVER_NODES /" $CONFIG_PATH
[ -n "$NO_OF_WORKER_NODES" ] && sed -i "s/no_of_worker_nodes = .*/no_of_worker_nodes = $NO_OF_WORKER_NODES /" $CONFIG_PATH
[ -n "$SERVER_FLAGS" ] && sed -i "s/server_flags\s*=\s*.*/server_flags = \"$SERVER_FLAGS\"/" $CONFIG_PATH
[ -n "$WORKER_FLAGS" ] && sed -i "s/worker_flags\s*=\s*.*/worker_flags = \"$WORKER_FLAGS\"/" $CONFIG_PATH
[ -n "$VOLUME_SIZE" ] && sed -i "s/volume_size\s*=\s*.*/volume_size = \"$VOLUME_SIZE\"/" $CONFIG_PATH
[ -n "$NODE_OS" ] && sed -i "s/node_os\s*=\s*.*/node_os = \"$NODE_OS\"/" $CONFIG_PATH
[ -n "$AWS_AMI" ] && sed -i "s/aws_ami\s*=\s*.*/aws_ami = \"$AWS_AMI\"/" $CONFIG_PATH
[ -n "$AWS_USER" ] && sed -i "s/aws_user\s*=\s*.*/aws_user = \"$AWS_USER\"/" $CONFIG_PATH

cat "$CONFIG_PATH"
exec sh ./scripts/test-runner.sh "$@"
