## Architecture

For better maintenance, readability and productivity we encourage max of separation of concerns and loose coupling between packages so inner packages should not depend on outer packages. "External" and "Outer" layer or dependency here in this context is considered any other package within the framework.

### Packages

```bash
./distros-test-framework
│
├── cmd/
│   └── qase/                       # internal programs.
│
├── config/                         # Configuration files and readers
│   ├── k3s.tfvars                  # Legacy Configuration varfiles for k3s
│   ├── rke2.tfvars                 # Legacy Configuration varfiles for rke2
│   └── reader.go
│
├── docs/                           # Documentation
│   ├── architecture.md
│   ├── qa-infra-integration.md
│   └── examples/
│
├── entrypoint/                     # Test suite entry points
│   ├── validatecluster/
│   ├── upgradecluster/
│   ├── createcluster/
│
├── infrastructure/                 # Infrastructure provisioning
│   ├── legacy/                     # Legacy Terraform modules
│   └── qainfra/                    # QA infrastructure modules
│
├── internal/                       # Core logic and packages
│   ├── pkg/                        # Reusable packages
│   ├── provisioning/               # Infrastructure provisioning logic
│   ├── resources/                  # Shared resources
│   └── report/                     # Test reporting
│
├── scripts/                        # Build and execution scripts.
│
└── workloads/                      # Test workloads and manifests
    ├── amd64/
    └── arm/
```

### Explanation

- **`cmd/`**

```
Act:                  Build programs.
Responsibility:       Provides CLI tools like qase integration, should not contain business logic
```

- **`config/`**

```
Act:                  Configuration management and file readers
Responsibility:       Handles configuration files and provides config reading utilities
```

- **`docs/`**

```
Act:                  Documentation and examples
Responsibility:       Contains all project documentation, architecture diagrams, and usage examples
```

- **`entrypoint/`**

```
Act:                  Test suite entry points and orchestration
Responsibility:       Orchestrates test execution, should not implement test logic but coordinate components
```

- **`infrastructure/`**

```
Act:                  Infrastructure-as-Code modules and configurations
Responsibility:       Provides Terraform/OpenTofu modules, should be provider-agnostic and reusable
```

- **`internal/`**

```
Act:                  Core business logic and internal packages
Responsibility:       Contains the main framework logic, should not depend on outer layers
```

- **`scripts/`**

```
Act:                  Build, deployment, and execution scripts
Responsibility:       Provides scripts for CI/CD, container builds, and test execution
```

- **`workloads/`**

```
Act:                  Kubernetes workloads and test manifests
Responsibility:       Contains YAML manifests for different architectures, totally independent
```
