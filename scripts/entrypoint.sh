#!/bin/sh

ENV_PATH="/go/src/github.com/rancher/distros-test-framework/config/.env"
[ -n "$ENV_PRODUCT" ] && sed -i "s/ENV_PRODUCT=.*/ENV_PRODUCT=$ENV_PRODUCT/" "$ENV_PATH"
[ -n "$ENV_TFVARS" ] && sed -i "s/ENV_TFVARS=.*/ENV_TFVARS=$ENV_TFVARS/" "$ENV_PATH"


CONFIG_PATH="/go/src/github.com/rancher/distros-test-framework/config/${ENV_PRODUCT}.tfvars"
[ -n "$ARCH" ] && sed -i "s/arch\s*=\s*.*/arch = \"$ARCH\"/" "$CONFIG_PATH"
[ -n "$INSTALL_VERSION" ] && sed -i "s/k3s_version\s*=\s*.*/k3s_version = \"$INSTALL_VERSION\"/" "$CONFIG_PATH"
[ -n "$INSTALL_VERSION" ] && sed -i "s/rke2_version\s*=\s*.*/rke2_version = \"$INSTALL_VERSION\"/" "$CONFIG_PATH"
[ -n "$INSTALL_MODE" ] && sed -i "s/install_mode\s*=\s*.*/install_mode = \"$INSTALL_MODE\"/" "$CONFIG_PATH"
[ -n "$K3S_CHANNEL" ] && sed -i "s/k3s_channel\s*=\s*.*/k3s_channel = \"$K3S_CHANNEL\"/" "$CONFIG_PATH"
[ -n "$RKE2_CHANNEL" ] && sed -i "s/rke2_channel\s*=\s*.*/rke2_channel = \"$RKE2_CHANNEL\"/" "$CONFIG_PATH"
[ -n "$NO_OF_SERVER_NODES" ] && sed -i "s/no_of_server_nodes = .*/no_of_server_nodes = $NO_OF_SERVER_NODES /" "$CONFIG_PATH"
[ -n "$NO_OF_WORKER_NODES" ] && sed -i "s/no_of_worker_nodes = .*/no_of_worker_nodes = $NO_OF_WORKER_NODES /" "$CONFIG_PATH"
[ -n "$SERVER_FLAGS" ] && sed -i "s/server_flags\s*=\s*.*/server_flags = \"$SERVER_FLAGS\"/" "$CONFIG_PATH"
[ -n "$WORKER_FLAGS" ] && sed -i "s/worker_flags\s*=\s*.*/worker_flags = \"$WORKER_FLAGS\"/" "$CONFIG_PATH"
[ -n "$VOLUME_SIZE" ] && sed -i "s/volume_size\s*=\s*.*/volume_size = \"$VOLUME_SIZE\"/" "$CONFIG_PATH"
[ -n "$NODE_OS" ] && sed -i "s/node_os\s*=\s*.*/node_os = \"$NODE_OS\"/" "$CONFIG_PATH"
[ -n "$AWS_AMI" ] && sed -i "s/aws_ami\s*=\s*.*/aws_ami = \"$AWS_AMI\"/" "$CONFIG_PATH"
[ -n "$AWS_USER" ] && sed -i "s/aws_user\s*=\s*.*/aws_user = \"$AWS_USER\"/" "$CONFIG_PATH"
[ -n "$DATASTORE_TYPE" ] && sed -i "s/datastore_type\s*=\s*.*/datastore_type = \"$DATASTORE_TYPE\"/" "$CONFIG_PATH"

awk '!/^#|^$|access_key|key_name|username|password|region|qa_space/' "$CONFIG_PATH"

exec bash ./scripts/test_runner.sh "$@"