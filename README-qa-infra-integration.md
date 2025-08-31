# QA Infrastructure Automation Integration

This document explains the integration of the `qa-infra-automation` repository into the `distros-test-framework` for standardized infrastructure provisioning and product deployment.

## Overview

The framework now supports two infrastructure providers:

- **Legacy**: Original Terraform modules (now in `infrastructure/legacy/`)
- **QA-Infra**: New standardized approach using `qa-infra-automation` with OpenTofu and Ansible

## Architecture

### Flow Diagram

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Docker Build  │ -> │  Infrastructure  │ -> │   Ansible       │
│   & Container   │    │  Provisioning    │    │   Deployment    │
│     Setup       │    │   (OpenTofu)     │    │   (K3s/RKE2)    │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                       │                       │
         v                       v                       v
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│ Environment     │    │ AWS Resources:   │    │ Kubernetes      │
│ Variables &     │    │ - EC2 Instances  │    │ Cluster Ready   │
│ SSH Keys        │    │ - Route53 DNS    │    │ for Testing     │
└─────────────────┘    │ - Security Groups│    └─────────────────┘
                       │ - SSH Key Pairs  │
                       └──────────────────┘
```

## Configuration

### Environment Variables (.env)

The framework uses environment variables from `config/.env`:

```bash
# Core Configuration
ENV_PRODUCT=k3s                    # Product: k3s or rke2  
INFRA_PROVIDER=qa-infra            # Provider: qa-infra or legacy
INSTALL_VERSION=v1.33.3-rc1+k3s1  # Kubernetes version to install

# AWS Credentials
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key

# SSH Configuration
ACCESS_KEY_LOCAL="/path/to/private/key.pem"  # Host path to private key

# Test Configuration
TEST_DIR=validatecluster
DESTROY=false
LOG_LEVEL=debug
```

### Infrastructure Configuration

#### Unified Configuration Files

- `infrastructure/qa-infra/vars.tfvars` - OpenTofu variables (used for both K3s and RKE2)
- `infrastructure/qa-infra/ansible/vars.yaml` - Ansible variables (used for both K3s and RKE2)

#### Key Variables in vars.tfvars

```hcl
user_id             = "your_username"
aws_region          = "us-east-2"
aws_ami             = "ami-01de4781572fa1285"  # Amazon Linux 2
instance_type       = "t3.xlarge"
aws_ssh_user        = "ec2-user"

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

### 1. Docker Environment Setup

```bash
# Build the container
make test-build-run
```

The `scripts/docker_run.sh` script:

- Builds the Docker image with Go, OpenTofu, and Ansible
- Mounts SSH keys dynamically based on `ACCESS_KEY_LOCAL`
- Passes environment variables into the container
- Sets up the testing environment

### 2. Infrastructure Provisioning (OpenTofu)

When `INFRA_PROVIDER=qa-infra`, the framework:

1. **Initializes OpenTofu** in `infrastructure/qa-infra/`
2. **Creates workspace** with timestamp (e.g., `dsf-20250830221057`)
3. **Provisions AWS resources** using remote modules from `qa-infra-automation`:
   - EC2 instances (master + worker)
   - Route53 DNS records
   - AWS key pairs
   - Security groups

**Key Files:**

- `infrastructure/qa-infra/main.tf` - References remote qa-infra modules
- `infrastructure/qa-infra/variables.tf` - Variable definitions
- `infrastructure/qa-infra/vars.tfvars` - Variable values

### 3. Product Deployment (Ansible)

The framework then:

1. **Clones Ansible playbooks** from the appropriate branch:
   - K3s: `add-k3s-support` branch
   - RKE2: `main` branch

2. **Creates inventory file** with provisioned instance IPs:

   ```ini
   [all]
   master ansible_host=3.128.18.246
   worker-0 ansible_host=3.144.221.160

   [all:vars]
   ansible_ssh_private_key_file=/path/to/key.pem
   ansible_user=ec2-user
   ansible_ssh_common_args=-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null
   ```

3. **Runs ansible-playbook** with:
   - Product-specific playbook (k3s-playbook.yml or rke2-playbook.yml)
   - Version from `INSTALL_VERSION` environment variable
   - Unified configuration from `ansible/vars.yaml`

### 4. Test Execution

Once the cluster is ready:

- Kubeconfig is available at the specified path
- Go tests run against the provisioned cluster
- Tests validate cluster functionality

## Usage Instructions

### Quick Start

1. **Set up environment**:

   ```bash
   cp config/.env.example config/.env
   # Edit config/.env with your AWS credentials and SSH key path
   ```

2. **Run K3s tests**:

   ```bash
   export INFRA_PROVIDER=qa-infra
   export ENV_PRODUCT=k3s
   make test-build-run
   ```

3. **Run RKE2 tests**:

   ```bash
   export INFRA_PROVIDER=qa-infra
   export ENV_PRODUCT=rke2
   make test-build-run
   ```

### Using Legacy Provider

To use the original approach:

```bash
export INFRA_PROVIDER=legacy  # or unset INFRA_PROVIDER
export ENV_PRODUCT=k3s
make test-build-run
```

### SSH Key Setup

1. **Ensure your private key exists**:

   ```bash
   ls -la /path/to/your/private/key.pem
   ```

2. **Generate public key if missing**:

   ```bash
   ssh-keygen -y -f /path/to/your/private/key.pem > /path/to/your/private/key.pem.pub
   ```

3. **Update .env file**:

   ```bash
   ACCESS_KEY_LOCAL="/absolute/path/to/your/private/key.pem"
   ```

## Debugging

### Enable Debug Logging

```bash
export LOG_LEVEL=debug
```

### Check Infrastructure State

```bash
# Inside container or locally with OpenTofu installed
cd infrastructure/qa-infra
tofu workspace list
tofu workspace select dsf-YYYYMMDDHHMMSS
tofu show
```

### Verify Ansible Inventory

The framework creates debug output showing:

- Node IPs discovered from OpenTofu state
- Inventory file content
- Ansible command being executed

### Common Issues

1. **SSH Key Permissions**:

   ```bash
   chmod 600 /path/to/your/private/key.pem
   ```

2. **AWS Credentials**:
   - Verify AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY
   - Check AWS CLI: `aws sts get-caller-identity`

3. **Docker Mount Issues**:
   - Ensure ACCESS_KEY_LOCAL points to a file, not directory
   - Verify the .pub file exists alongside the private key

## File Structure

```
distros-test-framework/
├── infrastructure/
│   ├── qa-infra/                 # New qa-infra approach
│   │   ├── main.tf              # References remote modules
│   │   ├── variables.tf         # Variable definitions
│   │   ├── vars.tfvars          # Unified variables (K3s & RKE2)
│   │   └── ansible/
│   │       └── vars.yaml        # Unified Ansible vars (K3s & RKE2)
│   └── legacy/                   # Original Terraform modules
├── config/
│   └── .env                      # Environment configuration
├── scripts/
│   ├── docker_run.sh            # Docker container management
│   └── test_runner.sh           # Test execution wrapper
├── shared/
│   └── infrastructure.go        # Infrastructure provisioning logic
└── entrypoint/
    └── validatecluster/         # Test suites
```

## Key Features

✅ **Dual Provider Support**: Switch between legacy and qa-infra with environment variable  
✅ **Unified Configuration**: Single config files work for both K3s and RKE2  
✅ **Dynamic SSH Handling**: SSH keys loaded from environment, no hardcoding  
✅ **Docker Integration**: Full containerization for CI/CD pipelines  
✅ **Environment-Driven**: All configuration via environment variables  
✅ **Version Flexibility**: Install any K3s/RKE2 version via INSTALL_VERSION  
✅ **Workspace Isolation**: Each run uses unique OpenTofu workspace  
✅ **DNS Integration**: Automatic Route53 DNS record creation  

## Contributing

When modifying the qa-infra integration:

1. **Test both providers**: Ensure legacy provider still works
2. **Test both products**: Verify K3s and RKE2 functionality  
3. **Environment variables**: Avoid hardcoding, use .env configuration
4. **Documentation**: Update this README with any new features or changes

## Troubleshooting

### Infrastructure Issues

- Check OpenTofu logs for provisioning errors
- Verify AWS permissions and quotas
- Ensure unique resource naming (handled automatically)

### Ansible Issues  

- Verify SSH connectivity to instances
- Check Ansible inventory file format
- Ensure proper key permissions and paths

### Container Issues

- Verify Docker build completes successfully
- Check mount paths and permissions
- Ensure environment variables are properly passed
