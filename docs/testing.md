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

The test tag `secretsEncryptMethod` is optional which is set to `both` by default, which performs both `prepare/rotate/reencrypt` and `rotate-keys` operations. Other values that is accepted are `classic`, which performs `prepare/rotate/reencrypt` operations and `rotate-keys` which performs only `rotate-keys` operations.
Additionally starting v1.30.12 and above, support for secretbox has been added only for `rotate-keys` and below can be added on the `server_flags` to test that feature.
```
secrets-encryption-provider: secretbox
```

Note/TODO: k3s external db fails working with etcd only node. Refer: https://docs.k3s.io/datastore/ha

## Validating Dual-Stack

- Required vars for `.env` file

```
TEST_DIR=dualstack
```

- Required vars for `*.tfvars` file
- `kubelet-arg: \n - node-ip=0.0.0.0` is required to be added to both server and worker flags if the public and private IPv6 IPs are same

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

## Validating IPv6 Only

- Required vars for `.env` file

```
ENV_MODULE=ipv6only
TEST_DIR=ipv6only
```

- Required vars for `*.tfvars` file

```
enable_public_ip     = false
enable_ipv6          = true
server_flags         = "cluster-cidr: <ipv6-cluster-cider>\nservice-cidr: <ipv6-service-cidr>"
worker_flags         = ""
no_of_bastion_nodes  = 1
bastion_subnets      = "<dual-stack-subnet>"
```
- Test package should be `ipv6only`
- AWS config (sg, vpc) is available only in US-WEST-1 region
- Split roles is not supported at this time (Future enhancement)
- RPM install is not supported at this time (Future enhancement)

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

### Using Tarball

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
TEST_TAG=tarball
TARBALL_TYPE=tar.zst
```
Note: `TARBALL_TYPE` can be either `tar.zst` (Recommended for speed) or `tar.gz`

#### For Jenkins

- `MODULE` **should be** `airgap`
- `TEST_DIR` **should** be `airgap`
- `TEST_TAGS` **should** include `-tags=tarball`

### Not supported/implemented currently for airgap:
- RPM installs for rke2
- ExternalDB setup
- Split roles
- ARM Architecture

## Validating Cluster Reset Restore Path

For patch validation test runs, we need an HA Setup and a single fresh instance:
3 servers
1 agent
1 fresh instance

- Required vars in `*.tfvars` file:

```
server_flags    = "" (unless using a cni)
worker_flags    = "" (unless using a cni)
```

#### For Local/Docker

- Required vars that should be exported anywhere locally:
```
AWS_ACCESS_KEY_ID=<KEY_ID>
AWS_SECRET_ACCESS_KEY=<KEY>
```
- Optional vars in `.env` file: these can be user configured, default will be used if not provided.
```
S3_BUCKET=distros_qa
S3_FOLDER=snapshots
```

### Not supported/implemented currently for cluster restore:
- Hardened Cluster Setup
- ExternalDB Setup
- Selinux Setup

## Validating Conformance Tests with Sonobuoy
 - Please note that the sonobuoy version has not been updated for a year and the functionality of sonobuoy is degrading with minor versions of k8s.
 - Full conformance tests done for patch validations should be 3 servers 1 agent minimum.
 - You can use the make file command `make test-run-sonobuoy` to run the conformance tests.
 - Additionally you can use `go test -timeout=140m -v -count=1 ./entrypoint/conformance/... --ginkgo.timeout=140m` you must extend the ginkgo timeout in addition to the go test timeout 
 - Required vars in `*.tfvars` file minimum conformance configuration: 
```
no_of_server_nodes = 1
no_of_worker_nodes = 1
```
- sonobuoy's output is becoming unreliable for status checks observe remaining count incorrect at 404.
 	sono status
          PLUGIN     STATUS   RESULT   COUNT                                PROGRESS
             e2e   complete   passed       1   Passed:  0, Failed:  0, Remaining:404
    systemd-logs   complete   passed       2
 Sonobuoy has completed. Use `sonobuoy retrieve` to get results


## Kill all - Uninstall tests
- Based on fixes here https://github.com/rancher/rke2/commit/613399eefdd581b137fc9bd7aeb7fb59ecdfdd8e introduced in v1.30.4 when running kill all script after this,
we are not directly umounting the data dir, which is the behavior tests scripts are expecting.

- Ideally we should run killall uninstall tests on versions above 1.30.4.

- Optionally we can pass `-killallUninstall true` to run kill all-uninstall tests on the end of ValidateCluster tests.

- Despite number of severs/agents those tests are always running only in one server/agent node if any, to avoid unecessary extra time and resources consumption.

## Selinux tests.
These tests are enabled in 3 test suites: 
1. Selinux test suite
2. Validate cluster test suite
3. killall test suite - in particular the uninstall test check only for selinux tests.

When using with the validate cluster test suite: 
- Use the flag `-selinux "${SELINUX_TEST}"` where SELINUX_TEST value is a boolean: `true` or `false`
- `server_flags` should have the value `'selinux: true'` in it
