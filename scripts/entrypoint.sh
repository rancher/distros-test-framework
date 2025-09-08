#!/bin/sh

ENV_PATH="/go/src/github.com/rancher/distros-test-framework/config/.env"
if [ "$PROVISIONER_MODULE" = "legacy" ] || [ -z "$PROVISIONER_MODULE" ]; then 
[ -n "$ENV_PRODUCT" ] && sed -i "s/ENV_PRODUCT=.*/ENV_PRODUCT=$ENV_PRODUCT/" "$ENV_PATH"
[ -n "$ENV_TFVARS" ] && sed -i "s/ENV_TFVARS=.*/ENV_TFVARS=$ENV_TFVARS/" "$ENV_PATH"

CONFIG_PATH="/go/src/github.com/rancher/distros-test-framework/config/${ENV_PRODUCT}.tfvars"
[ -n "$INSTALL_VERSION" ] && sed -i "s/k3s_version\s*=\s*.*/k3s_version = \"$INSTALL_VERSION\"/" "$CONFIG_PATH"
[ -n "$INSTALL_VERSION" ] && sed -i "s/rke2_version\s*=\s*.*/rke2_version = \"$INSTALL_VERSION\"/" "$CONFIG_PATH"
[ -n "$SSH_KEY_NAME" ] && sed -i "s/key_name\s*=\s*.*/key_name = \"$SSH_KEY_NAME\"/" "$CONFIG_PATH"
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
[ -n "$SSH_USER" ] && sed -i "s/aws_user\s*=\s*.*/aws_user = \"$SSH_USER\"/" "$CONFIG_PATH"
[ -n "$DATASTORE_TYPE" ] && sed -i "s/datastore_type\s*=\s*.*/datastore_type = \"$DATASTORE_TYPE\"/" "$CONFIG_PATH"
 
    awk '!/^#|^$|access_key|key_name|username|password|region|qa_space/' "$CONFIG_PATH"
else

echo "PROVISIONER_MODULE is using qainfra" 
CONFIG_PATH="/go/src/github.com/rancher/distros-test-framework/infrastructure/qainfra/vars.tfvars"
[ -n "$SSH_USER" ] && sed -i "s/aws_ssh_user\s*=\s*.*/aws_ssh_user = \"$SSH_USER\"/" "$CONFIG_PATH"
[ -n "$AWS_AMI" ] && sed -i "s/aws_ami\s*=\s*.*/aws_ami = \"$AWS_AMI\"/" "$CONFIG_PATH"

if [ -z "$NO_OF_SERVER_NODES" ]; then
    echo "ERROR: NO_OF_SERVER_NODES is required but not set or empty"
    exit 1
fi

echo "Building cluster configuration with $NO_OF_SERVER_NODES server nodes"

# start with at least one server node.
cat > /tmp/nodes_config.txt << EOF
nodes = [
  {
    count = $NO_OF_SERVER_NODES
    role  = ["etcd", "cp", "worker"]  
  }
EOF

# Only add worker nodes if NO_OF_WORKER_NODES is set AND greater than 0.
if [ -n "$NO_OF_WORKER_NODES" ] && [ "$NO_OF_WORKER_NODES" -gt 0 ]; then
    echo "Adding $NO_OF_WORKER_NODES worker nodes"
    cat >> /tmp/nodes_config.txt << EOF
,
  {
    count = $NO_OF_WORKER_NODES
    role  = ["worker"]
  }
EOF
else
    echo "NO_OF_WORKER_NODES not set or is 0 - deploying server-only cluster"
fi

echo "]" >> /tmp/nodes_config.txt
sed -i '/^nodes = \[/,/^\]/d' "$CONFIG_PATH"
cat /tmp/nodes_config.txt >> "$CONFIG_PATH"
rm /tmp/nodes_config.txt

echo "Cluster configuration updated successfully"
cat "$CONFIG_PATH"
fi

exec bash ./scripts/test_runner.sh "$@"