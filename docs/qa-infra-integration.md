# QA Infrastructure Automation Integration

High level overview of the integration of the `qainfra-automation` repository into the `distros-test-framework` for standardized infrastructure provisioning and product deployment.

## Overview

The distros test framework now supports two infrastructure modules providers:

- **Legacy**: Original Terraform modules (now in `infrastructure/legacy/` which will be deprecated)
- **QA-Infra**: New standardized approach using `qainfra-automation` with OpenTofu and Ansible (in `infrastructure/qainfra/`) giving a unified configuration for both K3s and RKE2,
allowing easy switching and adding new providers/provisioners implementations.

## Architecture

### Flow Diagram
```
┌─────────────────┐    ┌─────────────────── ┐    ┌─────────────────┐    ┌─────────────────┐
│   Environment   │ -> │   OpenTofu(for now)│ -> │   Node Config   │ -> │   Ansible       │
│     Setup       │    │  Infrastructure    │    │   Extraction    │    │   Deployment    │
│  (entrypoint.sh)│    │   Provisioning     │    │ (buildCluster)  │    │  (K3s/RKE2)     │
└─────────────────┘    └─────────────────── ┘    └─────────────────┘    └─────────────────┘
         │                       │                       │                       │
         v                       v                       v                       v
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ • .env vars     │    │ Resources:       │    │ • Extract IPs   │    │ • Dynamic       │
│                 │    │ - Instances/VMs  │    │ • Infer roles   │    │   Inventory     │
│ • vars.tfvars   │    │ - DNS records    │    │ • Populate      │    │ • Remote        │
└─────────────────┘    │ - Security Groups│    │   ServerIPs     │    │   Playbooks     │
                       │ - Load Balancer  │    │ • Build cluster │    │                 │
                       └──────────────────┘    │   config        │    │                 │ 
                                               └─────────────────┘    └─────────────────┘
```

## QA Infrastructure Pipeline Flow

The qainfra provisioning pipeline follows these sequential steps:

### Pipeline Steps

1. **Environment Setup**            - Configure variables and container environment
2. **setupDirectories**             - Create temporary directories for Tofu and Ansible
3. **prepareTerraformFiles**        - Copy and configure OpenTofu files
4. **executeInfraProvisioner**      - Provision infrastructure resources
5. **buildClusterConfig**           - Extract node information from Tofu state
6. **setupAnsibleEnvironment**      - Clone remote playbooks and setup inventory
7. **executeAnsiblePlaybook**       - Deploy K3s/RKE2 cluster
8. **addTofuOutputsToConfig**       - Add final outputs to cluster config


### Key Components

- **Remote Repository**: `qa-infra-automation` - Contains Ansible playbooks and scripts
- **Dynamic Inventory**: Uses `cloud.terraform.terraform_provider` plugin
- **Template Substitution**: `envsubst` for environment variable injection
- **Node Roles**: Supports split roles (etcd, cp, worker) and combined roles (inside ansible playbook not distros test framework[for now])

## Configuration

### Environment Variables (.env)

It needs some new environment variables from `config/.env`:

```bash
# FRAMEWORK VARS
PROVISIONER_MODULE=qainfra                                  # Provider: qainfra or legacy
QA_INFRA_PROVIDER=aws                                       # qainfra module to use: aws, vsphere, harvester, etc. ( for now only aws is supported )
PROVISIONER_TYPE=opentofu                                   # Provisioner type: opentofu(tf), cluster api, etc.
RESOURCE_NAME={your-local-resource-name}                    # Unique resource name prefix for AWS resources (e.g., "mytest123")

# SSH Configuration
SSH_USER=ec2-user                                           # SSH user to connect to instances/vms.
SSH_LOCAL_KEY_PATH="/path/to/private/aws_key.pem"           # Host path to private key

# Product Configuration
NODE_OS=sles15                                              # Node OS to use: sles15, rhel8, rhel9, etc.
CNI=calico                                                  # RKE2 only for now CNI plugin: calico, canal, flannel, etc.
CHANNEL=stable                                              # Channel to use: stable, latest, etc.
ARCH=amd64                                                  # Architecture to use: amd64, arm64, etc.
```

### Environment Variables per test

The vars above apply to every run. Each suite (selected by `TEST_DIR`, sometimes refined
by `TEST_TAG`) needs a few extra vars. Uppercase names are canonical for qainfra; most also
accept the legacy lowercase alias.

```bash
# Common to every suite
ENV_PRODUCT=rke2                                            # rke2 or k3s
INSTALL_VERSION=v1.35.0+rke2r1                              # baseline version to install
INSTALL_CHANNEL=testing                                    # testing | latest | stable
INSTALL_MODE=INSTALL_RKE2_VERSION                          # INSTALL_RKE2_VERSION | INSTALL_RKE2_COMMIT (k3s: INSTALL_K3S_*)
SERVER_FLAGS="write-kubeconfig-mode: 644\nselinux: true"   # \n-separated rke2/k3s config keys
WORKER_FLAGS="selinux: true"
DESTROY=false                                              # true tears the cluster down after the test
TEST_DIR=<suite>                                           # see per-suite below
```

```bash
# Cluster topology
NO_OF_SERVER_NODES=1                                        # simple mode (all-roles servers + workers)
NO_OF_WORKER_NODES=1
# split roles (set SPLIT_ROLES=true, then counts by role combo)
SPLIT_ROLES=false
ETCD_ONLY_NODES=0
ETCD_CP_NODES=0
ETCD_WORKER_NODES=0
CP_ONLY_NODES=0
CP_WORKER_NODES=0
```

```bash
# upgradecluster  (TEST_DIR=upgradecluster)
TEST_TAG=upgradesuc                                        # upgradesuc | upgrademanual | upgradereplacement
SUC_UPGRADE_VERSION=v1.36.1+rke2r1                         # upgradesuc
INSTALL_VERSION_OR_COMMIT=v1.36.1+rke2r1                   # upgrademanual / upgradereplacement
CHANNEL=stable
```

```bash
# versionbump  (TEST_DIR=versionbump)
TEST_TAG=components                                        # components | flannel | cilium | calico | canal | multus | versionbump
EXPECTED_VALUE=cilium,cni                                  # comma-sep; count per tag (k3s components=10, rke2 components=7, single-CNI=1-2)
VALUE_UPGRADED=...                                         # same count, post-upgrade (omit to skip the upgrade check)
EXPECTED_CHARTS_VALUE=cilium                               # required non-empty; only asserted for rke2 charts
INSTALL_VERSION_OR_COMMIT=v1.36.1+rke2r1                   # upgrade target
CMD="..."                                                  # only for TEST_TAG=versionbump; must be empty for the other tags
```

```bash
# deployrancher  (TEST_DIR=deployrancher)
CHARTS_REPO_URL=https://releases.rancher.com/server-charts/latest
CHARTS_REPO_NAME=rancher-latest                            # must match the channel in the URL
CHARTS_VERSION=v2.14.0                                     # rancher chart version (helm normalizes the leading v)
CERT_MANAGER_VERSION=v1.19.4                               # must match ^v\d+\.\d+\.\d+$
CHARTS_ARGS=bootstrapPassword=admin                        # comma-separated helm --set args, unquoted
```

```bash
# clusterrestore  (TEST_DIR=clusterrestore)
S3_BUCKET=distrosqa
S3_FOLDER=snapshots
CHANNEL=stable
```

```bash
# conformance  (TEST_DIR=conformance)
SONOBUOY_VERSION=0.57.3                                     # optional

# nvidia  (TEST_DIR=nvidia) — needs a GPU-capable AMI/instance
NVIDIA_VERSION=580.95.05
```

```bash
# External datastore (any suite): set DATASTORE_TYPE=external, else etcd is used
DATASTORE_TYPE=external
EXTERNAL_DB=postgres                                       # postgres | mysql | mariadb | aurora-mysql
EXTERNAL_DB_VERSION=18.3
DB_GROUP_NAME=default.postgres18
EXTERNAL_DB_NODE_TYPE=db.t3.medium
DB_USERNAME=adminuser
DB_PASSWORD=admin1234
```

```bash
# CIS hardening — qainfra applies the rke2 CIS sysctls + etcd user automatically for any
# node whose server_flags/worker_flags contain "profile: cis":
SERVER_FLAGS="profile: cis\nselinux: true"
WORKER_FLAGS="profile: cis\nselinux: true"

# Rancher on a CIS cluster ALSO needs the custom PSA file delivered and referenced, or
# Rancher's cattle-system pods are rejected by the strict default pod-security:
SERVER_FLAGS="profile: cis\nselinux: true\npod-security-admission-config-file: /etc/rancher/rke2/custom-psa.yaml"
OPTIONAL_FILES=/etc/rancher/rke2/custom-psa.yaml,https://gist.githubusercontent.com/rancher-max/e1c728805b1e5aae8b547b075261bb56/raw/pod_security_config.yaml
```

### Infrastructure Configuration

#### Configuration Files

- `infrastructure/qainfra/vars.tfvars`          - OpenTofu variables (used for both K3s and RKE2)
- `infrastructure/qainfra/ansible/vars.yaml`    - Ansible variables  (used for both K3s and RKE2) ( for easier management inside distros for now it is being handled in code )

#### Key Variables in vars.tfvars

```hcl
user_id             = "distros-qa-"
aws_hostname_prefix  = "distros-qa-"

aws_region          = "us-east-2"
aws_route53_zone    =  "qa.rancher.space"
aws_security_group  = "sg-08e8243a8cfbea8a0"
aws_vpc             =  "vpc-bfccf4d7"
aws_volume_size     = "50"
aws_subnet          = "subnet-ee8cac86"
aws_volume_type     = "gp3"
aws_ami             = "ami-01de4781572fa1285"
instance_type       = "t3.xlarge"
aws_ssh_user        = "ec2-user"

public_ssh_key      = "{leave-empty-to-auto-generate}"
aws_access_key      = "{leave-empty-to-use-env-var}"
aws_secret_key      = "{leave-empty-to-use-env-var}"


# Node configuration
nodes = [
  {
    count = 1 
    role  = ["etcd", "cp", "worker"]  # All-in-one master node
  },
  {
    count = 1 
    role  = ["worker"]                # Dedicated worker node
  }
]
```

## How It Works

### 1. Infrastructure Provisioning (OpenTofu)

When `PROVISIONER_MODULE=qainfra`, the framework:

- After having env vars or files done ( .env, vars.tfvars )
- After your reach the entrypoint ( test_suites ) in any form ( docker,local,Jenkins... )

1. **Initializes OpenTofu** in `infrastructure/qainfra/`
2. **Creates workspace** with timestamp (e.g., `dsf-20250830221057`)
3. **Provisions resources** using remote modules from `qainfra-automation`:

### 2. Product Deployment (Ansible)

The framework then:

1. **Clones Ansible playbooks**
2. **Creates inventory file** with provisioned instance IPs:

   ```ini
   [all]
   master ansible_host=3.128.18.246
   worker-0 ansible_host=3.144.221.160

   [all:vars]
   ansible_ssh_private_key_file=/path/to/aws_key.pem
   ansible_user=ec2-user
   ansible_ssh_common_args=-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null
   ```

3. **Runs ansible-playbook** with:
   - Product-specific playbook (k3s-playbook.yml or rke2-playbook.yml)
   - Version from `INSTALL_VERSION` environment variable
   - Unified configuration from `ansible/vars.yaml`

## Troubleshooting

### Common debug and verification steps that might help

### Check Infrastructure State

```bash
# Inside container or locally with OpenTofu installed
cd infrastructure/qainfra  cd /tmp/qainfra-tofu-dsf-YYYYMMDDHHMMSS
tofu workspace list
tofu workspace select dsf-YYYYMMDDHHMMSS
tofu show

 cd /tmp/qainfra-tofu-dsf-YYYYMMDDHHMMSS
 tofu show
```

### Verify Ansible Inventory

The framework creates debug output showing:

```bash
cd /tmp/qainfra-ansible
cat inventory.yml
```
```bash
ansible-inventory --list
```

```bash
ansible-inventory --graph
```