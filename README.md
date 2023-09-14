## Distros Framework - Acceptance Tests

The acceptance tests are a customizable way to create clusters and perform validations on them such that the requirements of specific features and functions can be validated.

- It relies on [Terraform](https://www.terraform.io/) to provide the underlying cluster configuration.
- It uses [Ginkgo](https://onsi.github.io/ginkgo/) and [Gomega](https://onsi.github.io/gomega/) as assertion framework.

## Architecture
- For better maintenance, readability and productivity we encourage max of separation of concerns and loose coupling between packages so inner packages should not depend on outer packages

### Packages:
```bash
./distros-test-framework
│
├── entrypoint
│   └───── Entry for tests execution, separated by test runs and test suites
│
├── modules
│   └───── Terraform modules and configurations
│
│── scripts
│    └───── Scripts needed for overall execution
│
├── shared
│    └───── auxiliary and reusable functions
│
├── pkg
│   └───── Place where resides the logic and services for it
│
│── workloads
│   └───── Place where resides workloads to use inside tests
```

### Explanation:

- `Pkg`
```
    Testcase:
  
Act:                  Acts as an innermost layer where the main logic (test implementation) is handled.
Responsibility:       Encapsulates test logic and should not depend on any outer layer
```

- `Entrypoint`
````
Act:                  Acts as the one of the outer layer to receive the input to start test execution
Responsibility:       Should not implement any logic and only focus on orchestrating
````

- `Modules`
```
Act:                  Acts as the infra to provide the terraform modules and configurations
Responsibility:       Only provides indirectly for all, should not need the knowledge of any test logic or have dependencies from internal layers.
```

- `Scripts`
```
Act:                  Acts as a provider for scripts needed for overall execution
Responsibility:       Should not need knowledge of or "external" dependencies at all and provides for all layers.
```

- `Shared`
```
Act:                  Acts as an intermediate module providing shared, reusable and auxiliary functions
Responsibility:       Should not need knowledge of or "external" dependencies at all and provides for all layers.
```

- `Workloads`
````
Act:                  Acts as a provider for test workloads
Responsibility:       Totally independent of any other layer and should only provide
````

#### PS: "External" and "Outer" layer or dependency here in this context is considered any other package within the framework.

-------------------


### `Template Bump Version Model`

- We have a template model interface for testing bump versions, the idea is to provide a simple and direct way to test bump of version using go test tool.


```You can test that like:```

- Adding one version or commit and ran some commands on it and check it against respective expected values then upgrade and repeat the same commands and check the respective new (or not) expected values.



```How can I do that?```

- Step 1: Add your desired first version or commit that you want to use on `local.tfvars` file on the vars `k3s_version` and `install_mode`
- Step 2: Have the commands you need to run and the expected output from them
- Step 3: Have a version or commit that you want to upgrade to.
- Step 4: On the TestConfig field you can add another test case that we already have or a newly created one.
- Step 5: You can add a standalone workload deploy if you need
- Step 6: Just fill the go test or make command with your required values
- Step 7: Run the command and wait for results.
- Step 8: (WIP) Export your customizable report.

-------------------

Available arguments to create your command with examples:
````
- $ -cmd "kubectl describe pod -n kube-system local-path-provisioner- : | grep -i Image"
- $ -expectedValue "v0.0.21"
- $ -expectedValueUpgrade "v0.0.24"
- $ -installVersionOrCommit INSTALL_K3S_COMMIT=257fa2c54cda332e42b8aae248c152f4d1898218
- $ -deployWorkload true
- $ -testCase "TestLocalPathProvisionerStorage"
- $ -workloadName "bandwidth-annotations.yaml"
- $ -description "Description of your test"

* All non-boolean arguments is comma separated in case you need to send more than 1.

* If you need to separate another command to run as a single here , separate those with " : " as this example:
-cmd "kubectl describe pod -n kube-system local-path-provisioner- :  | grep -i Image"

* All flags are optional but -cmd and -expectedValue but if you want to use one of them, you need to use all of them.

````

Example of an execution with multiple values on k3s:
```bash
go test -timeout=45m -v -tags=versionbump  ./entrypoint/versionbump/... \
-cmd "/var/lib/rancher/k3s/data/current/bin/cni, kubectl get pod test-pod -o yaml : | grep -A2 annotations, k3s -v" \
-expectedValue "v1.2.0-k3s1,1M, v1.26" \
-expectedValueUpgrade "v1.2.0-k3s1,1M, v1.27" \
-installVersionOrCommit INSTALL_K3S_VERSION=v1.27.2+k3s1 \
-testCase "TestServiceClusterIP, TestLocalPathProvisionerStorage" \
-deployWorkload=true \
-workloadName "bandwidth-annotations.yaml"
````
Example of an execution with multiple values on rke2:
```bash
go test -v -timeout=45m -tags=versionbump ./entrypoint/versionbump/... \
-cmd "(find /var/lib/rancher/rke2/data/ -type f -name runc -exec {} --version \\;), rke2 -v"  \
-expectedValue "v1.9.3, v1.25.9+rke2r1"  \
-expectedValueUpgrade "v1.10.1, v1.26.4-rc1+rke2r1" \
-installVersionOrCommit INSTALL_RKE2_VERSION=v1.25.9+rke2r1 \
-testCase "TestServiceClusterIP, TestIngress" \
-deployWorkload true \
-workloadName "ingress.yaml" \
-description "Testing ingress and service cluster ip"

```



Example of an execution with less args on k3s:
````bash
go test -timeout=45m -v -tags=versionbump  ./entrypoint/versionbump/... \
-cmd "/var/lib/rancher/k3s/data/current/bin/cni, kubectl get pod test-pod -o yaml : | grep -A2 annotations, k3s -v"  \
-expectedValue "v1.2.0-k3s1,1M, v1.26"  \
-expectedValueUpgrade "v1.2.0-k3s1,1M, v1.27" \
-installVersionOrCommit INSTALL_K3S_VERSION=v1.27.2+k3s1 \
````

Example of an execution with less args on rke2:
````bash
go test -timeout=45m -v -tags=versionbump  ./entrypoint/versionbump/... \
-cmd "(find /var/lib/rancher/rke2/data/ -type f -name runc -exec {} --version \\;)"  \
-expectedValue "1.1.7"  \
-expectedValueUpgrade "1.10.1" \
-installVersionOrCommit INSTALL_RKE2_VERSION=v1.27.2+rke2r1 \
````



#### We also have this on the `makefile` to make things easier to run just adding the values, please see bellow on the makefile section


-----
#### Testcase naming convention:
- All tests should be placed under `./testcase/<TESTNAME>`.
- All test functions should be named: `Test<TESTNAME>`.


## Running

- Before running the tests, you should creat file in `./config/{product}.tfvars`. There is some information there to get you started, but the empty variables should be filled in appropriately per your AWS environment.

- Also before running on the config.yaml add your product name and tfvars product name

- Please make sure to export your correct AWS credentials before running the tests. e.g:
```bash
export AWS_ACCESS_KEY_ID=<YOUR_AWS_ACCESS_KEY_ID>
export AWS_SECRET_ACCESS_KEY=<YOUR_AWS_SECRET_ACCESS_KEY>
```

- The local.tfvars split roles section should be strictly followed to not cause any false positives or negatives on tests

- For running tests with "etcd" cluster type, you should add the value "etcd" to the variable "datastore_type" , also you need have those variables at least empty:
```
- external_db       
- external_db_version
- instance_class  
- db_group_name
```

- For running with external db you need the same variables above filled in with the correct data and also datastore_type= ""

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


###  Run with `Makefile` locally:
```bash
- On the first run with make and docker please delete your .terraform folder, terraform.tfstate and terraform.hcl.lock file

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
````
### Examples to run locally:
````
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
````

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
````

### Your first steps - prep your setup to run tests:
1. Fork your own git copy, clone it and create a branch in your local git repo.
2. Create the following files in config directory path: 

    a. `k3s.tfvars`: Copy over and edit the `k3s.tfvars.example` file

    b. `rke2.tfvars`: Copy over and edit the `rke2.tfvars.example` file

    c. Run these commands:

    ````
    touch config/.ssh/aws_key.pem; chmod 600 config/.ssh/aws_key.pem;
    ````
    Copy over contents of `jenkins-rke-validation.pem` file, or you own `.pem` file content. An example file can be found with permissions set.
    There should be a corresponding AWS key pair in AWS cloud. Make sure the name of which pair, you have used, is added into the tfvars file in the next step.
    Ensure permissions to this file is set so no one else has access to the same.

    d. Edit the following vars in the tfvars file:
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
    password      = "<password>"   
    db_username   = "<db_user>"
    db_password   = "<db_password>"   
    ```
    e. Create config/config.yaml file with contents: 
    ```
    ENV_PRODUCT: k3s
    ENV_TFVARS: k3s.tfvars
    ```
3. Export the following variables:
    ```
    export AWS_ACCESS_KEY_ID=xxx
    export AWS_SECRET_ACCESS_KEY=xxxx
    export ACCESS_KEY_LOCAL=/PATH/TO/distros-test-framework/config/.ssh/aws_key.pem
   ```
You are now set to use make commands or the go test commands 

### Working with M2 chip on macOS:

Docker and most virtualization solutions don't work well with M2 chip. 

Solution: Use `lima+nerdctl` commands instead. 
1. Install `lima`: 
    ```
    brew install lima
    ```
2. Can `cd` to your work directory, say: 
    ```
    cd ${HOME}/distro-test-framework
    ```
3. Start a lima VM default instance (we can use current config option). It has nerdctl pre-installed and ready to use. 
    ``` 
    limactl start 
    ```
4. Access lima VM shell with your current directory mounted already.  
    ``` 
    lima
    ```
    Remember to make any code changes before starting VM/building your image.
5.  Build your image and run the same:
    ```
    nerdctl build -t k3s . -f ./scripts/Dockerfile.build
    nerdctl run -it --rm k3s
    ```
6. Export variables in your lima VM:
    ```
    export AWS_ACCESS_KEY_ID=xxx
    export AWS_SECRET_ACCESS_KEY=xxxx
    export ACCESS_KEY_LOCAL=/go/src/github.com/rancher/distros-test-framework/config/.ssh/aws_key.pem
    ```
7. We are now ready to run a sample test: 
    ```
    cd entrypoint
    go test -timeout=45m -v ./createcluster/...
    ```
8. FYI. To delete unused container/image:
    ```
    nerdctl container prune
    nerdctl image rm < image name >
    ```
