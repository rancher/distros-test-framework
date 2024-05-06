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

### Secret-Encryption tests Setup Requirements/Assumptions
We need a split role setup for this test:
1 Etcd ONLY node
2 Control Plane ONLY node
1 Agent node

To set this up, please use the following in the tfvars file: 

```
no_of_server_nodes = 0  # This is for all roles server - etcd + control plane
no_of_worker_nodes = 1  # Agent node
split_roles        = true
etcd_only_nodes    = 1  # etcd only node count
etcd_cp_nodes      = 0 
etcd_worker_nodes  = 0
cp_only_nodes      = 2  # control plane only node count
cp_worker_nodes    = 0
# Numbers 1-6 correspond to: all-roles (1), etcd-only (2), etcd-cp (3), etcd-worker (4), cp-only (5), cp-worker (6).
role_order         = "2,5,5"
```

Please set the server_flags in .tfvars file:
```
server_flags   = "secrets-encryption: true\n"
```

Note/TODO: k3s external db fails working with etcd only node. Refer: https://docs.k3s.io/datastore/ha


### Dual-Stack Testing

- Required vars for `*.tfvars` file
- `kubelet-arg: \n - node-ip=0.0.0.0` is required to be added to both server and worker flags if the public and private IPs are same

```
enable_public_ip     = true
enable_ipv6          = true
server_flags         = "cluster-cidr: <ipv4-cluster-cidr>,<ipv6-cluster-cider>\nservice-cidr: <ipv4-service-cidr>,<ipv6-service-cidr>\nkubelet-arg: \n - node-ip=0.0.0.0\n"
worker_flags         = "\nkubelet-arg: \n - node-ip=0.0.0.0\n"
no_of_bastion_nodes  = 1
bastion_subnets      = "<dual-stack-subnet>"
```
- Test package should be `dualstack`
- AWS config (sg, vpc) is available only in US-WEST-1 region
- Split roles is not supported at this time (Future enhancement)
- Reorder IP is not supported at this time (Future enhancement)

### Rancher Deployment Testing

- Required flags in `*.tfvars` file
```
create_lb: true
```

#### For executing locally via docker
- Optional flags that can be added in `.env` file. Default values are set on entrypoint/deployrancher/rancher_suite_test.go
```
CERTMANAGERVERSION=v1.13.3
CHARTSVERSION=v2.7.12
CHARTSREPONAME=<helm repo name>
CHARTSREPOURL=<helm chart repo url>
CHARTSARGS=bootstrapPassword=admin,replicas=1 #(Comma separated helm chart args)
RANCHERVERSION=v2.7.12
```

#### For executing in Jenkins or locally without docker
- Optional flags that can be passed as test parameters. Default values are set on entrypoint/deployrancher/rancher_suite_test.go
```
go test -timeout=30m -v -tags=deployrancher ./entrypoint/deployrancher/... \
-certManagerVersion v1.13.3 \
-chartsVersion v2.7.12 \
-chartsRepoName <helm repo name> \
-chartsRepoUrl <helm chart repo url> \
-chartsArgs bootstrapPassword=admin,replicas=1 \
-rancherVersion v2.7.12
```

#### For Rancher v2.7.12, need to add these additional helm args
```
chartsArgs rancherImage=<image or url>,extraEnv[0].name=CATTLE_AGENT_IMAGE,extraEnv[0].value=<image or url>-agent:v2.7.12
```
