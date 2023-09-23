#!/bin/bash
# This script defines which role this node will be and writes that to a file
# that is readable by rke2

if [ $# != 8 ]; then
  echo "Usage: rke2_node_roles.sh node_index role_order all_role_nodes etcd_only_nodes etcd_cp_nodes etcd_worker_nodes cp_only_nodes cp_worker_nodes"
  exit 1
fi

node_index=$(($1+1))
role_order=$2
all_role_nodes=$3
etcd_only_nodes=$4
etcd_cp_nodes=$5
etcd_worker_nodes=$6
cp_only_nodes=$7
cp_worker_nodes=$8

# Set the desired role into an array based on the index
read -r -a order_array <<< "$role_order"
role_array=()
for order_num in "${order_array[@]}"; do
  case "$order_num" in
    1)
     role_array=$'\n' read -r -d '' -a role_array < <(printf "all-role-nodes %.0s " $(seq 1 "$all_role_nodes") && printf '\0')
      ;;
    2)
    role_array=$'\n' read -r -d '' -a role_array < <(printf "etcd-only %.0s " $(seq 1 "$etcd_only_nodes") && printf '\0')
      ;;
    3)
      role_array=$'\n' read -r -d '' -a role_array < <(printf "etcd-cp %.0s " $(seq 1 "$etcd_cp_nodes") && printf '\0')
      ;;
    4)
      role_array=$'\n' read -r -d '' -a role_array < <(printf "etcd-worker %.0s " $(seq 1 "$etcd_worker_nodes") && printf '\0')
      ;;
    5)
      role_array=$'\n' read -r -d '' -a role_array < <(printf "cp-only %.0s " $(seq 1 "$cp_only_nodes") && printf '\0')
      ;;
    6)
      role_array=$'\n' read -r -d '' -a role_array < <(printf "cp-worker %.0s " $(seq 1 "$cp_worker_nodes") && printf '\0')
      ;;
  esac
done

# Get role based on which node is being created
role="${role_array[$node_index]}"
echo "Writing config for a ${role} node."

# Write config
mkdir -p /etc/rancher/rke2/config.yaml.d
if [[ "$role" == "etcd-only" ]]
then
cat << EOF > /etc/rancher/rke2/config.yaml.d/role_config.yaml
disable-apiserver: true
disable-controller-manager: true
disable-scheduler: true
node-taint:
  - node-role.kubernetes.io/etcd:NoExecute
EOF

elif [[ "$role" == "etcd-cp" ]]
then
cat << EOF > /etc/rancher/rke2/config.yaml.d/role_config.yaml
node-taint:
  - node-role.kubernetes.io/control-plane:NoSchedule
  - node-role.kubernetes.io/etcd:NoExecute
EOF
cat << EOF > /tmp/.control-plane
true
EOF

elif [[ "$role" == "etcd-worker" ]]
then
cat << EOF > /etc/rancher/rke2/config.yaml.d/role_config.yaml
disable-apiserver: true
disable-controller-manager: true
disable-scheduler: true
EOF

elif [[ "$role" == "cp-only" ]]
then
cat << EOF > /etc/rancher/rke2/config.yaml.d/role_config.yaml
disable-etcd: true
node-taint:
  - node-role.kubernetes.io/control-plane:NoSchedule
EOF
cat << EOF > /tmp/.control-plane
true
EOF

elif [[ "$role" == "cp-worker" ]]
then
cat << EOF > /etc/rancher/rke2/config.yaml.d/role_config.yaml
disable-etcd: true
EOF
cat << EOF > /tmp/.control-plane
true
EOF

else
cat << EOF > /tmp/.control-plane
true
EOF
fi