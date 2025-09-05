# Infrastructure Directory

This directory contains both legacy and new infrastructure provisioning approaches for the distros test framework.

## Structure

```
infrastructure/
├── legacy/                    # Old modules/ approach (Terraform-based)
│   ├── k3s/
│   ├── rke2/
│   └── ...
├── qa-infra/                  # New standardized approach (remote modules + temporary Ansible)
│   ├── main.tf               # OpenTofu config using remote qa-infra-automation modules
│   ├── variables.tf          # Variable definitions
│   ├── vars.tfvars          # Sample configuration
│   └── ansible/
│       └── vars.yaml        # Ansible variables for RKE2 installation
├── qa-infra.env              # Environment variables for local usage
├── qa-infra-docker.env       # Environment variables for Docker/Jenkins
└── README.md                 # This file
```

## Usage

### QA Infrastructure Automation Approach

The new approach uses [remote modules from qa-infra-automation](https://github.com/rancher/qa-infra-automation) and downloads only the Ansible playbooks needed:

  **Set environment variable:**

   ```bash
   export INFR_PROVIONER="qa-infra"
   # Or source the env file:
   source infrastructure/qa-infra.env
   ```
