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

## Validating Rancher Day-1 Operations

`entrypoint/rancherday1` deploys Rancher like `deployrancher`, then on the same product (`ENV_PRODUCT`):

1. points Rancher's `rke-metadata-config` at the data.json under test and waits for `k8sVersion` in `/v1-<product>-release/releases`;
2. screenshots the create-cluster **Kubernetes Version** dropdown (must list `k8sVersion`);
3. provisions a custom cluster on `k8sVersion` with the `customNodes` layout;
4. takes an etcd snapshot of it, deletes a marker ConfigMap, restores the snapshot and checks the marker is back;
5. installs `k8sPreviousVersion` (a released version) as a standalone cluster with the `importNodes` layout and imports it;
6. screenshots the imported cluster's edit dropdown (must list `k8sVersion`) and upgrades it to `k8sVersion` through Rancher's imported-cluster version management.

The dashboard hides older patches of a minor version, so the create dropdown is only expected to list `k8sVersion`.

Node layouts are comma-separated `roles:count` groups. Roles are `etcd`, `controlplane` (or `cp`) and `worker`, joined with `+`. A layout holds at most 10 nodes.

| Flag (env) | Default | Example |
|---|---|---|
| `-customNodes` (`CUSTOM_NODES`) | `etcd:3,controlplane:2,worker:1` | `etcd+cp+worker:3` (3 all-role nodes) |
| `-importNodes` (`IMPORT_NODES`) | `etcd+controlplane:3,worker:1` | `etcd+cp:1` (single server) |

- `customNodes` must include at least one etcd, one controlplane and one worker node.
- `importNodes` must start with `etcd+controlplane` servers and may add `worker` agents. The first server starts with `cluster-init`; the other servers and agents join it over its private IP.
- The join token is written to `/etc/rancher/<product>/config.yaml` over SSH stdin, so it never appears on a command line.

Steps run with `ContinueOnFailure`: the import and its upgrade still run when a custom-cluster step fails, and steps whose prerequisite failed are skipped.

- `kdmSource` takes a raw data.json `https` URL or a `rancher/kontainer-driver-metadata` PR URL, which is pinned to the PR head commit.
- Both versions must be listed in that data.json for the product; this is checked before any infrastructure is created.
- Downstream nodes are extra EC2 instances (10 with the default layouts) that use the cluster's AMI, instance type, subnet, security group and SSH key. With `-destroy true` they are terminated before the infrastructure is destroyed. An aborted run can leak them: they are tagged `<RESOURCE_NAME>-day1-*-distros-qa`.
- Needs Chromium or Chrome (both test images include `chromium`) and a connected environment, because Rancher downloads data.json and the nodes download install scripts. Air-gap is not supported.

Artifacts are written to `artifacts/rancherday1/`. Jenkins archives them as build artifacts.
- `screenshots/NN-*.png`: the dropdowns, plus every cluster after each successful operation.
- `debug/NN/`: written per failed spec. It holds Rancher logs, provisioning/control-plane/machine/snapshot objects, events, downstream node journals, cluster-agent logs, a failure screenshot and the browser console.

Credentials: the browser authenticates with an API session token set as an `httpOnly` cookie, so no password is typed or rendered. A screenshot is refused when the page shows the bootstrap password, the session token or a registration token. Registration and import commands are sent over SSH stdin, so their tokens never appear in logs or errors.

Every suite flag can also be set in `config/.env`; a flag given on the command line wins. `CHANNEL` only matters when no version is pinned. `INSTALL_VERSION` pins the Rancher local cluster and `K8S_VERSION` pins the RC, so `CHANNEL=stable` is fine when testing an RC.

| `config/.env` | Flag | Purpose |
|---|---|---|
| `CHARTS_VERSION`, `CHARTS_REPO_NAME`, `CHARTS_REPO_URL`, `CHARTS_ARGS` | `-chartsVersion` … | Rancher chart to install |
| `CERT_MANAGER_VERSION` | `-certManagerVersion` | cert-manager version |
| `KDM_SOURCE` | `-kdmSource` | data.json URL or KDM PR |
| `K8S_VERSION` | `-k8sVersion` | version under test: custom cluster install version and imported upgrade target |
| `K8S_PREVIOUS_VERSION` | `-k8sPreviousVersion` | released version the imported cluster starts on |
| `CUSTOM_NODES`, `IMPORT_NODES` | `-customNodes`, `-importNodes` | node layouts |
| `DESTROY` | `-destroy` | keep or tear down all infrastructure |
| `RANCHER_FORCE_PASSWORD_CHANGE` | `-rancherForcePasswordChange` | default `true`; see below |

With `DESTROY=false` the run leaves Rancher up for manual checks. It revokes its own API token, sets the admin account to "must change password", and writes `artifacts/rancherday1/rancher-access.txt` (mode 0600). That file holds the Rancher URL, the admin login and the kept downstream EC2 instance IDs; logs show only its path. Your first UI login with that password makes you choose a new one. Leave the chart's `bootstrapPassword` unset to get a random admin password.

#### For executing locally via docker
```
CHARTS_VERSION=2.16.0-7c25ce9-head
CHARTS_REPO_NAME=optimus
CHARTS_REPO_URL=https://charts.optimus.rancher.io/server-charts/latest
KDM_SOURCE=https://github.com/rancher/kontainer-driver-metadata/pull/2248
K8S_VERSION=v1.37.3-rc1+k3s1
K8S_PREVIOUS_VERSION=v1.37.2+k3s1
```

#### For executing in Jenkins or locally without docker
```
go test -timeout=240m -v -count=1 ./entrypoint/rancherday1/... \
-chartsVersion 2.16.0-7c25ce9-head \
-chartsRepoName optimus \
-chartsRepoUrl https://charts.optimus.rancher.io/server-charts/latest \
-kdmSource https://github.com/rancher/kontainer-driver-metadata/pull/2248 \
-k8sVersion v1.37.3-rc1+k3s1 \
-k8sPreviousVersion v1.37.2+k3s1 \
-destroy true
```

Sample map (charts repo `https://charts.optimus.rancher.io/server-charts/latest`). Each KDM PR carries the RCs of one product only:

| Rancher (`chartsVersion`) | Product | `kdmSource` PR | `k8sVersion` | `k8sPreviousVersion` |
|---|---|---|---|---|
| 2.16.0-7c25ce9-head | k3s | rancher/kontainer-driver-metadata#2248 | v1.37.3-rc1+k3s1 | v1.37.2+k3s1 |
| 2.15.4-ce34ff1-head | k3s | rancher/kontainer-driver-metadata#2247 | v1.36.7-rc1+k3s1 | v1.36.6+k3s1 |
| 2.14.8-5739c20-head | k3s | rancher/kontainer-driver-metadata#2246 | v1.35.11-rc1+k3s1 | v1.35.10+k3s1 |
| 2.16.0-7c25ce9-head | rke2 | rancher/kontainer-driver-metadata#2251 | v1.37.3-rc1+rke2r1 | v1.37.2+rke2r1 |
| 2.15.4-ce34ff1-head | rke2 | rancher/kontainer-driver-metadata#2250 | v1.36.7-rc1+rke2r1 | v1.36.6+rke2r2 |
| 2.14.8-5739c20-head | rke2 | rancher/kontainer-driver-metadata#2249 | v1.35.11-rc1+rke2r1 | v1.35.10+rke2r1 |

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

#### With the qainfra provisioner (`PROVISIONER_MODULE=qainfra`)

- The legacy `*.tfvars` keys above (`enable_public_ip`, `no_of_bastion_nodes`, `bastion_subnets`) are **not** used. Set `MODULE=airgap` and a qainfra-format `vars.tfvars`; the framework adds `airgap_setup=true` and `bastion={enabled=true}` for the `cluster_nodes` module and installs the product offline through `ansible/airgap/airgap-playbook.yml` in qa-infra-automation (`QA_INFRA_REF`, resolved once to a commit).
- Registry credentials: by default user `testuser` with a random per-run password for the throwaway bastion registry. To override, set `REGISTRY_USERNAME` / `REGISTRY_PASSWORD` in the environment (optional Jenkins credential `AIRGAP_REGISTRY_CREDENTIALS`); `-registryUsername` / `-registryPassword` in `TEST_ARGS` are rejected on qainfra.
- ARM is supported on qainfra (`ARCH=arm64`, `BASTION_INSTANCE_TYPE=t4g.large`); Windows agents, RPM installs and `profile: cis` / `protect-kernel-defaults` are not yet.
- Local run: `make test-tarball` / `make test-private-registry` / `make test-system-default-registry` with the `.env` above plus `PROVISIONER_MODULE=qainfra`.

### Not supported/implemented currently for airgap:
- RPM installs for rke2
- ExternalDB setup
- Split roles
- ARM Architecture (legacy provisioner only; supported on qainfra)

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

#### With the qainfra provisioner (`PROVISIONER_MODULE=qainfra`)

- The legacy `*.tfvars` keys above (`enable_public_ip`, `no_of_bastion_nodes`, `bastion_subnets`) are **not** used. Set `MODULE=airgap` and a qainfra-format `vars.tfvars`; the framework adds `airgap_setup=true` and `bastion={enabled=true}` for the `cluster_nodes` module and installs the product offline through `ansible/airgap/airgap-playbook.yml` in qa-infra-automation (`QA_INFRA_REF`, resolved once to a commit).
- Registry credentials: by default user `testuser` with a random per-run password for the throwaway bastion registry. To override, set `REGISTRY_USERNAME` / `REGISTRY_PASSWORD` in the environment (optional Jenkins credential `AIRGAP_REGISTRY_CREDENTIALS`); `-registryUsername` / `-registryPassword` in `TEST_ARGS` are rejected on qainfra.
- ARM is supported on qainfra (`ARCH=arm64`, `BASTION_INSTANCE_TYPE=t4g.large`); Windows agents, RPM installs and `profile: cis` / `protect-kernel-defaults` are not yet.
- Local run: `make test-tarball` / `make test-private-registry` / `make test-system-default-registry` with the `.env` above plus `PROVISIONER_MODULE=qainfra`.

### Not supported/implemented currently for airgap:
- RPM installs for rke2
- ExternalDB setup
- Split roles
- ARM Architecture (legacy provisioner only; supported on qainfra)

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
