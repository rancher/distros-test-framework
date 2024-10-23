## [Qase-Reporter](../pkg/qase/README.md)
## [Qase-Patch-Validation-Create](#qase-patch-validation-create)


# Qase-Patch-Validation-Create

### Description
This job aims to automatically create a patch validation in Qase for a given project and test plan ID.

### Required Parameters
All variables are already defined or set in run time.
- `QASE_API_TOKEN`: Qase API token to authenticate with the Qase API.               # Set as secret on GH actions
- `QASE_PROJECT_CODE`: Qase project code to create the patch validation for.        # Set as variable inside yaml.
- `QASE_TEST_PLAN_ID`: Qase test plan ID to create the patch validation for.        # Set as variable inside yaml.
- `QASE_TAG`: Tag to filter the team the patch validation.                          # Set as variable inside yaml.
- `QASE_MILESTONE`: Milestone from the given patch.                                 # Set as variable inside yaml.


### Usage
- Go to gh actions https://github.com/rancher/distros-test-framework/actions
- Click on `run workflow` and select the `Qase-Patch-Validation-Create` workflow.
- Fill in the required parameters which are versions and rcs target.
- The job based on parameters above will create the title,description and milestone for the run.