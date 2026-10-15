## Distros Framework - Acceptance Tests

The acceptance tests are a customizable way to create clusters and perform validations on them such that the requirements of specific features and functions can be validated.

- The supported CI provisions clusters with [QA-INFRA](./docs/qa-infra-integration.md) (OpenTofu); the legacy
  [Terraform](https://www.terraform.io/) provisioner is still available and is the default when `PROVISIONER_MODULE` is unset.
- It uses [Ginkgo](https://onsi.github.io/ginkgo/) and [Gomega](https://onsi.github.io/gomega/) as assertion framework.

See `docs/` for any more specific information and examples.
- [Architecture](./docs/architecture.md)
- [Conventions](./docs/conventions.md)
- [Development & Getting Started](./docs/development.md)
- [Version Bump Template Model](./docs/version_bump_template.md)
- [Example Configs](./docs/examples/)
