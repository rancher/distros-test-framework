## Development


### First Steps - prep your setup to run tests:
1. Fork your own git copy, clone it and create a branch in your local git repo.

   Please note that example files can be found under `config/examples` directory for your reference.
2. Create the following files in 'config' directory path: 

    a. `k3s.tfvars`: Copy over `docs/examples/k3s.tfvars.example` file into `config/k3s.tfvars` and edit it.

    b. `rke2.tfvars`: Copy over `docs/examples/rke2.tfvars.example` file into `config/rke2.tfvars` and edit it.

3.  Edit the following vars in the tfvars file:

    i. Generic variables:
    ```
    resource_name = "<name of aws resource you will create - your prefix name>"
    key_name      = "jenkins-rke-validation"   # or your own aws key pair for the .pem file you used in previous step. 
    access_key    = "/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem"
    ```
    ii. AWS related mandatory variable values: 
    ```
    vpc_id             = "<vpc_id>"
    subnets            = "<subnet_id>"
    sg_id              = "<sg_id>"
    iam_role           = "<iam_role>"
    aws_ami            = "<ami_id>"
    windows_aws_ami    = "<ami-id>"    # rke2.tfvars file only
    ```
    iii. Sensitive variables to edit in tfvars file:
    ```
    password      = "<password>"   # Note - this is needed if we run test locally using go test. Not needed from jenkins runs. 
    db_username   = "<db_user>"
    db_password   = "<db_password>"   
    ```
4. Create `config/config.yaml` file with contents: 
   ```
   ENV_PRODUCT: k3s
   ENV_TFVARS: k3s.tfvars
   ```
   Please use `examples/config.yaml.example` for reference. 
   Note to set the "{{PRODUCT}}" value to k3s or rke2 as in the example above.

5.  Export the following variables:
    ```
    export AWS_ACCESS_KEY_ID=xxx
    export AWS_SECRET_ACCESS_KEY=xxxx
    export ACCESS_KEY_LOCAL=/PATH/TO/distros-test-framework/config/.ssh/aws_key.pem
    ```
6. Run these commands:

    ````
    cd config; mkdir .ssh; touch config/.ssh/aws_key.pem; chmod 600 config/.ssh/aws_key.pem;
    ````
   Copy over contents of `jenkins-rke-validation.pem` file, or you own `.pem` file content. An example file can be found with permissions set.
   There should be a corresponding AWS key pair in AWS cloud. Make sure the name of which pair, you have used, is added into the tfvars file `key_name` variable.
   Also, note the `access_key` var in tfvars file, is referring to your .pem file path we create in this step.
   Ensure permissions to this file is set so no one else has access to the same.

You are now set to use make commands or the go test commands 

### Environment Setup
- Before running the tests, you should create a file in `config/{product}.tfvars`. There is some information in the examples here to get you started. **DO NOT MODIFY THE EXAMPLES.** Only add your file to the `config` directory. You can copy and paste the example files there, but the empty variables should be filled in appropriately per your AWS environment.

- Also before running, in `config/config.yaml` add your product name and tfvars product name.

- Please make sure to export your correct AWS credentials before running the tests. e.g:
```bash
export AWS_ACCESS_KEY_ID=<YOUR_AWS_ACCESS_KEY_ID>
export AWS_SECRET_ACCESS_KEY=<YOUR_AWS_SECRET_ACCESS_KEY>
```

- The local.tfvars split roles section should be strictly followed to not cause any false positives or negatives on tests

- For running tests with "etcd" cluster type, you should add the value "etcd" to the variable "datastore_type". You also need have those variables at least empty:
```
- external_db       
- external_db_version
- instance_class  
- db_group_name
```

- For running with external db you need the same variables above filled in with the correct data and also datastore_type="[YOUR DB TYPE]"

### RKE2 Only

- For running tests on a RKE2 cluster with Windows agent, additional vars are required to create the Windows instance in AWS and join the node as agent
- In `rke2.tfvars` file, add the below vars:
```
server_flags                = "cni: calico"
windows_ec2_instance_class  = "<use t3.xlarge or higher>"
windows_aws_ami             = "<windows ami>"
no_of_windows_worker_nodes  = <count of Windows node>
```

#### NOTES: 
- The sonobuoy test runs outside of the cluster, so if running the test locally, user have to clean up sonobouy manually. 
- The MixedOS test is not supported with split-roles (TBA later) or Hardened cluster (Not supported in Windows)

### Test Execution

Tests can be run individually per package:
```bash
go test -timeout=45m -v ./entrypoint/${PACKAGE_NAME}/...

go test -timeout=45m -v ./entrypoint/$PACKAGE_NAME/...

go test -timeout=45m -v -tags=upgrademanual ./entrypoint/upgradecluster/... -installVersionOrCommit v1.25.8+rke2r1

go test -timeout=45m -v -tags=upgradesuc ./entrypoint/upgradecluster/... -upgradeVersion v1.25.8+rke2r1
```

Test flags:
```
${installVersionOrCommit} type of installation (version or commit) + desired value

-installVersionOrCommit version or commit

${upgradeVersion} version to upgrade to as SUC

-upgradeVersion v1.26.2+rke2r1
```

Test tags rke2:
```
 -tags=upgradesuc
```


### Run with `Makefile` locally:

On the first run each time with make and docker please delete your .terraform folder, terraform.tfstate and terraform.hcl.lock file

```bash

Args:
*Most of args are optional so you can fit to your use case.

- ${IMGNAME}               append any string to the end of image name
- ${TAGNAME}               append any string to the end of tag name
- ${ARGNAME}               name of the arg to pass to the test
- ${ARGVALUE}              value of the arg to pass to the test
- ${TESTDIR}               path to the test directory 
- ${TESTFILE}              path to the test file
- ${TAGTEST}               name of the tag function from suite ( -tags=upgradesuc or -tags=upgrademanual )
- ${TESTCASE}              name of the testcase to run
- ${DEPLOYWORKLOAD}        true or false to deploy workload
- ${CMD}                   command to run
- ${VALUE}                 value to check on host
- ${INSTALLTYPE}           type of installation (version or commit) + desired value
- &{WORKLOADNAME}          name of the workload to deploy
- &{DESCRIPTION}           description of the test

Commands: 
$ make test-env-up                     # create the image from Dockerfile.build
$ make test-run                        # runs create and upgrade cluster by passing the argname and argvalue
$ make test-env-down                   # removes the image and container by prefix
$ make test-env-clean                  # removes instances and resources created by testcase
$ make test-logs                       # prints logs from container the testcase
$ make test-complete                   # clean resources + remove images + run testcase
$ make test-create                     # runs create cluster test locally
$ make test-upgrade                    # runs upgrade cluster test locally
$ make test-version-bump               # runs version bump test locally
$ make test-run                        # runs create and upgrade cluster by passing the argname and argvalue
$ make remove-tf-state                 # removes acceptance state dir and files
$ make test-suite                      # runs all testcase locally in sequence not using the same state
$ make vet-lint                        # runs go vet and go lint
```

### Examples with docker:
```
- Create an image tagged
$ make test-env-up TAGNAME=ubuntu

- Run upgrade cluster test with `${IMGNAME}` and  `${TAGNAME}`
$ make test-run IMGNAME=2 TAGNAME=ubuntu TESTDIR=upgradecluster INSTALLTYPE=1.26.2+k3s1

- Run create and upgrade cluster just adding `INSTALLTYPE` flag to upgrade
$ make test-run INSTALLTYPE=257fa2c54cda332e42b8aae248c152f4d1898218

- Run version bump test upgrading with commit id
$ make test-run IMGNAME=x \
TAGNAME=y \
TESTDIR=versionbump \
CMD="k3s --version, kubectl get image..." \
VALUE="v1.26.2+k3s1, v0.0.21" " \
INSTALLTYPE=257fa2c54cda332e42b8aae248c152f4d1898218 \
TESTCASE=TestLocalPathProvisionerStorage \
DEPLOYWORKLOAD=true \
WORKLOADNAME="someWorkload.yaml"
```

### Examples to run locally:
```
- Run create cluster test:
$ make test-create

- Run upgrade cluster test:
$ make test-upgrade-manual INSTALLTYPE=257fa2c54cda332e42b8aae248c152f4d1898218

- Run bump version with go test:
$go test -timeout=45m -v -tags=versionbump  ./entrypoint/versionbump/... \
-cmd "/var/lib/rancher/k3s/data/current/bin/cni, kubectl get pod test-pod -o yaml ; | grep -A2 annotations, k3s -v" \
-expectedValue "CNI plugins plugin v1.2.0-k3s1,1M, v1.26" \
-expectedValueUpgrade "CNI plugins plugin v1.2.0-k3s1,1M, v1.27" \
-installVersionOrCommit INSTALL_K3S_VERSION=v1.27.2+k3s1 \
-testCase "TestServiceClusterIP, TestLocalPathProvisionerStorage" \
-deployWorkload true \
-workloadName "bandwidth-annotations.yaml"

 - Logs from test
$ make tf-logs IMGNAME=1

- Run lint for a specific directory
$ make vet-lint TESTDIR=upgradecluster
```

### Running tests in parallel:

- You can play around and have a lot of different test combinations like:
```
- Build docker image with different TAGNAME="OS`s" + with different configurations( resource_name, node_os, versions, install type, nodes and etc) and have unique "IMGNAMES"

- And in the meanwhile run also locally with different configuration while your dockers TAGNAME and IMGNAMES are running
```

### In between tests:
```
- If you want to run with same cluster do not delete ./modules/{product}/terraform.tfstate + .terraform.lock.hcl file after each test.

- if you want to use new resources then make sure to delete the ./modules/{product}/terraform.tfstate + .terraform.lock.hcl file if you want to create a new cluster.
```

### Debugging
````
To focus individual runs on specific test clauses, you can prefix with `F`. For example, in the [create cluster test](../tests/acceptance/entrypoint/createcluster_test.go), you can update the initial creation to be: `FIt("Starts up with no issues", func() {` in order to focus the run on only that clause.
Or use break points in your IDE.
````

### Custom Reporting: WIP

### Debugging:
````
The cluster and VMs can be retained after a test by passing `-destroy=false`. 
To focus individual runs on specific test clauses, you can prefix with `F`. For example, in the [create cluster test](../tests/terraform/cases/createcluster_test.go), you can update the initial creation to be: `FIt("Starts up with no issues", func() {` in order to focus the run on only that clause.