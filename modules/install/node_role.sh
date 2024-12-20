#!/bin/bash
# This script defines which role this node will be and writes that to a file
# that is readable by rke2 or k3s

if [ $# != 9 ]; then
  echo "Usage: node_roles.sh node_index role_order all_role_nodes etcd_only_nodes etcd_cp_nodes etcd_worker_nodes cp_only_nodes cp_worker_nodes product datastore_type"
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
product=$9
if [ -z $10 ]; then
  datastore_type="etcd"
else
  datastore_type=$10
fi
echo "Datastore type: $datastore_type"

role_config_yaml_path="/etc/rancher/${product}/config.yaml.d/role_config.yaml"
# Set the desired role into an array based on the index
order_array=($(echo "$role_order" | tr ',' '\n'))
role_array=()
for order_num in "${order_array[@]}"; do
  case "$order_num" in
    1)
      role_array+=($(printf "all-roles %.0s " $(seq 1 "$all_role_nodes")))
      ;;
    2)
      role_array+=($(printf "etcd-only %.0s " $(seq 1 "$etcd_only_nodes")))
      ;;
    3)
      role_array+=($(printf "etcd-cp %.0s " $(seq 1 "$etcd_cp_nodes")))
      ;;
    4)
      role_array+=($(printf "etcd-worker %.0s " $(seq 1 "$etcd_worker_nodes")))
      ;;
    5)
      role_array+=($(printf "cp-only %.0s " $(seq 1 "$cp_only_nodes")))
      ;;
    6)
      role_array+=($(printf "cp-worker %.0s " $(seq 1 "$cp_worker_nodes")))
      ;;
  esac
done

# Get role based on which node is being created
role="${role_array[$node_index]}"
echo "Writing config for a ${role} node."

# Write config
mkdir -p "/etc/rancher/${product}/config.yaml.d"
if [[ "$role" == "etcd-only" ]]
then
cat << EOF > "${role_config_yaml_path}"
disable-apiserver: true
disable-controller-manager: true
disable-scheduler: true
node-taint:
  - node-role.kubernetes.io/etcd:NoExecute
node-label:
  - role-etcd=true
EOF

elif [[ "$role" == "etcd-cp" ]]
then
cat << EOF > "${role_config_yaml_path}"
node-taint:
  - node-role.kubernetes.io/control-plane:NoSchedule
  - node-role.kubernetes.io/etcd:NoExecute
node-label:
  - role-etcd=true
  - role-control-plane=true
EOF
cat << EOF > /tmp/.control-plane
true
EOF

elif [[ "$role" == "etcd-worker" ]]
then
cat << EOF > "${role_config_yaml_path}"
disable-apiserver: true
disable-controller-manager: true
disable-scheduler: true
node-label:
  - role-etcd=true
  - role-worker=true
EOF

elif [[ "$role" == "cp-only" ]]
then
cat << EOF > "${role_config_yaml_path}"
disable-etcd: true
node-taint:
  - node-role.kubernetes.io/control-plane:NoSchedule
node-label:
  - role-control-plane=true
EOF
cat << EOF > /tmp/.control-plane
true
EOF

elif [[ "$role" == "cp-worker" ]]
then
cat << EOF > "${role_config_yaml_path}"
disable-etcd: true
node-label:
  - role-control-plane=true
  - role-worker=true
EOF
cat << EOF > /tmp/.control-plane
true
EOF

else
  if [[ "$datastore_type" == "etcd" ]]
  then
  cat << EOF > "${role_config_yaml_path}"
node-label:
  - role-etcd=true
  - role-control-plane=true
  - role-worker=true
EOF
  else
  cat << EOF > "${role_config_yaml_path}"
node-label:
  - role-control-plane=true
  - role-worker=true
EOF
cat << EOF > /tmp/.control-plane
true
EOF
fi