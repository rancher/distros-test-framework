### Setup related Information 

### Cert Rotate tests Setup Requirements/Assumptions

We need a split role setup for this test:
1 Etcd ONLY node
1 Control Plane ONLY node
1 Agent node

To set this up, please use the following in the tfvars file: 

```
no_of_server_nodes = 0  # This is for all roles server - etcd + control plane
no_of_worker_nodes = 1  # Agent node
split_roles        = true
etcd_only_nodes    = 1  # etcd only node count
etcd_cp_nodes      = 0 
etcd_worker_nodes  = 0
cp_only_nodes      = 1  # control plane only node count
cp_worker_nodes    = 0
# Numbers 1-6 correspond to: all-roles (1), etcd-only (2), etcd-cp (3), etcd-worker (4), cp-only (5), cp-worker (6).
role_order         = "2,5"
```

The role_order determines the order of nodes in the server ip array that will get returned in the factory cluster object. 
server1 -> etcd only
server2 -> control plane only
agent1 ->  agent/worker node

Note/TODO: k3s external db fails working with etcd only node. Refer: https://docs.k3s.io/datastore/ha
