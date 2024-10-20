# Test Setup Information/Requirements 

## Validating Cert Rotate

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

## Validating Secrets-Encryption

For patch validation test runs, we need a split role setup for this test:
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
role_order         = "2,5"
```
Please note, we can also run this test on a regular HA setup - 3 all-roles server, 1 worker node. (without split roles)

Please set the server_flags in .tfvars file for k3s:
```
server_flags   = "secrets-encryption: true\n"
```

For versions 1.26 and 1.27 - we run the traditional tests only: prepare/rotate/reencrypt (TEST_TYPE gets set to 'classic' in env var. We use this to determine which tests to run.)
For versions 1.28 and greater - we run both the traditional tests and new method - rotate-keys (TEST_TYPE gets set to 'both' in env var)

Note/TODO: k3s external db fails working with etcd only node. Refer: https://docs.k3s.io/datastore/ha

## Validating Dual-Stack

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

## Validating Rancher Deployment

- Required flags in `*.tfvars` file
```
create_lb: true
```

#### For executing locally via docker
- Optional flags that can be added in `.env` file. Default values are set on `entrypoint/deployrancher/rancher_suite_test.go`
```
CERT_MANAGER_VERSION=v1.16.1
CHARTS_VERSION=v2.7.12
CHARTS_REPO_NAME=<charts repo name>
CHARTS_REPO_URL=<charts repo url>
CHARTS_ARGS=bootstrapPassword=admin,replicas=1 #(Comma separated chart args)
```

#### For executing in Jenkins or locally without docker
- Optional flags that can be passed as test parameters. Default values are set on `entrypoint/deployrancher/rancher_suite_test.go`
```
go test -timeout=30m -v -tags=deployrancher ./entrypoint/deployrancher/... \
-certManagerVersion v1.13.3 \
-chartsVersion v2.7.12 \
-chartsRepoName <charts repo name> \
-chartsRepoUrl <charts repo url> \
-chartsArgs bootstrapPassword=admin,replicas=1
```

#### For Rancher v2.7.12, need to add these additional charts args
```
chartsArgs rancherImage=<image or url>,extraEnv[0].name=CATTLE_AGENT_IMAGE,extraEnv[0].value=<image or url>-agent:v2.7.12
```

## Validating with kubeconfig file

- Please note that you also need to update on the `*.tfvars` the var `aws_user` with the correct one that was used to create the cluster.
- Required variables in `.env` file
```
KUBE_CONFIG=<kubeconfig file base64-encoded>
BASTION_IP=<bastion public ip> when testing Dual-Stack
```

## Validating Reboot Instances

- Required vars in `*.tfvars` file:
```
 create_eip =   true
```
- Optional vars for in `.env` file: Will be to not release EIPs after the test, so you can reuse the kubeconfig file.
Please note that using this option set to `false` you will need to manually release the EIPs.
 ```
 RELEASE_EIP=false
 ```

## Validating Airgap Cluster 

### Using Private Registry

- Required vars in `*.tfvars` file:
```
enable_public_ip     = false
enable_ipv6          = false
no_of_bastion_nodes  = 1
bastion_subnets      = "<ipv4-subnet>"
```
#### For local/docker

- Required vars in `.env` file: `ENV_MODULE` stores the terraform module dir under /modules that will be used to create the airgapped clusters

```
ENV_MODULE=airgap
TEST_DIR=airgap
TEST_TAG=privateregistry
```
- Optional vars in `.env` file: `IMAGE_REGISTRY_URL`, `REGISTRY_USERNAME` and `REGISTRY_PASSWORD` can be user configured, default will be used if not provided
```
IMAGE_REGISTRY_URL=registry_url
REGISTRY_USERNAME=testuser
REGISTRY_PASSWORD=testpass432
```

#### For Jenkins

- `MODULE` **should** be `airgap`
- `TEST_DIR` **should** be `airgap`
- `TEST_TAGS` **should** include `-tags=privateregistry` and **as optional** may include `-registryUsername testuser -registryPassword testpass432 -imageRegistryUrl registry_url`

### Using System Default Registry

- Required vars in `*.tfvars` file:
```
enable_public_ip     = false
enable_ipv6          = false
no_of_bastion_nodes  = 1
bastion_subnets      = "<ipv4-subnet>"
```
#### For local/docker

- Required vars in `.env` file: `ENV_MODULE` stores the terraform module dir under /modules that will be used to create the airgapped clusters

```
ENV_MODULE=airgap
TEST_DIR=airgap
TEST_TAG=systemdefaultregistry
```
- Optional vars in `.env` file: `IMAGE_REGISTRY_URL`, `REGISTRY_USERNAME` and `REGISTRY_PASSWORD` can be user configured, default will be used if not provided
```
IMAGE_REGISTRY_URL=registry_url
REGISTRY_USERNAME=testuser
REGISTRY_PASSWORD=testpass432
```

#### For Jenkins

- `MODULE` **should be** `airgap`
- `TEST_DIR` **should** be `airgap`
- `TEST_TAGS` **should** include `-tags=systemdefaultregistry` and **as optional**, include `-registryUsername testuser -registryPassword testpass432 -imageRegistryUrl registry_url`

### Not supported/implemented currently for airgap:
- RPM installs for rke2
- ExternalDB setup
- Split roles
- ARM Architecture