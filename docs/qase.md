## [Qase-Reporter](../pkg/qase/README.md)
## [Qase-Patch-Validation-Create](#qase-patch-validation-create)
## [Usefull links](#usefull-links)
## [Test cases Definition](#test-cases-definition)


# Qase-Patch-Validation-Create

### Description
This job aims to automatically create Test Runs in Qase to perform distros patch/minor validation for a given project code and test plan ID.

### All needed variables are already defined or set in run time.

- `QASE_API_TOKEN`: Qase API token to authenticate with the Qase API.
- `QASE_PROJECT_CODE`: Qase project code where to create the patch validation.
- `QASE_TEST_PLAN_ID`: Qase test plan ID used to create the patch validation.
- `QASE_TAG`: Tag to filter the team the patch validation.

### Usage
- Go to gh actions https://github.com/rancher/distros-test-framework/actions
- Click on `run workflow` and select the `Qase-Patch-Validation-Create` workflow.
- Fill in the required parameters with targeted rcs.
- Based on parameters above, the workflow will create the title, description and milestone for the test run in Qase.

# Usefull links
- [Qase API documentation](https://developers.qase.io/reference/introduction-to-the-qase-api)
- [Qase API token](https://app.qase.io/user/api/token)

# Test cases Definition
- [Test cases - K3S](https://app.qase.io/plan/K3SRKE2/15)
- [Test cases - RKE2](https://app.qase.io/plan/K3SRKE2/14)


