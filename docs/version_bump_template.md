## Template Bump Version Model

We have a template model interface for testing version bumps. The idea is to provide a simple and direct way to test when a version of a packaged component in either k3s or rke2 is bumped.

The test can be created by adding one version or commit, run some commands on it, and check it against respective expected values then upgrade and repeat the same commands and check the respective new (or not) expected values.


### How can I do that?

- Step 1: Add your desired first version or commit that you want to use on `local.tfvars` file on the vars `k3s_version` and `install_mode`
- Step 2: Have the commands you need to run and the expected output from them
- Step 3: Have a version or commit that you want to upgrade to.
- Step 4: On the TestConfig field you can add another test case that we already have or a newly created one.
- Step 5: You can add a standalone workload deploy if you need
- Step 6: Just fill the go test or make command with your required values
- Step 7: Run the command and wait for results.
- Step 8: (WIP) Export your customizable report.

Available arguments to create your command with examples:
```
- $ -cmd "kubectl describe pod -n kube-system local-path-provisioner- : | grep -i Image"
- $ -expectedValue "v0.0.21"
- $ -expectedValueUpgrade "v0.0.24"
- $ -installVersionOrCommit INSTALL_K3S_COMMIT=257fa2c54cda332e42b8aae248c152f4d1898218
- $ -applyWorkload true
- $ -deleteWorkload true
- $ -testCase "TestLocalPathProvisionerStorage"
- $ -workloadName "bandwidth-annotations.yaml"
- $ -description "Description of your test"
```

* All non-boolean arguments are comma separated in case you need to send more than 1.

* If you need to separate another command to run as a single here, separate those with " : " as this example:
-cmd "kubectl describe pod -n kube-system local-path-provisioner- :  | grep -i Image"

* All flags are optional except for -cmd and -expectedValue but if you want to use one of them, you need to use all of them.


Example of an execution with multiple values on k3s:
```bash
go test -timeout=45m -v -tags=versionbump  ./entrypoint/versionbump/... \
-cmd "/var/lib/rancher/k3s/data/current/bin/cni, kubectl get pod test-pod -o yaml : | grep -A2 annotations, k3s -v" \
-expectedValue "v1.2.0-k3s1,1M, v1.26" \
-expectedValueUpgrade "v1.2.0-k3s1,1M, v1.27" \
-installVersionOrCommit INSTALL_K3S_VERSION=v1.27.2+k3s1 \
-testCase "TestServiceClusterIP, TestLocalPathProvisionerStorage" \
-applyWorkload=true \
-deleteWorkload=false \
-workloadName "bandwidth-annotations.yaml"
```
Example of an execution with multiple values on rke2:
```bash
go test -v -timeout=45m -tags=versionbump ./entrypoint/versionbump/... \
-cmd "(find /var/lib/rancher/rke2/data/ -type f -name runc -exec {} --version \\;), rke2 -v"  \
-expectedValue "v1.9.3, v1.25.9+rke2r1"  \
-expectedValueUpgrade "v1.10.1, v1.26.4-rc1+rke2r1" \
-installVersionOrCommit INSTALL_RKE2_VERSION=v1.25.9+rke2r1 \
-testCase "TestServiceClusterIP, TestIngress" \
-applyWorkload true \
-deleteWorkload true \
-workloadName "ingress.yaml" \
-description "Testing ingress and service cluster ip"
```



Example of an execution with less args on k3s:
```bash
go test -timeout=45m -v -tags=versionbump  ./entrypoint/versionbump/... \
-cmd "/var/lib/rancher/k3s/data/current/bin/cni, kubectl get pod test-pod -o yaml : | grep -A2 annotations, k3s -v"  \
-expectedValue "v1.2.0-k3s1,1M, v1.26"  \
-expectedValueUpgrade "v1.2.0-k3s1,1M, v1.27" \
-installVersionOrCommit INSTALL_K3S_VERSION=v1.27.2+k3s1 \
```

Example of an execution with less args on rke2:
```bash
go test -timeout=45m -v -tags=versionbump  ./entrypoint/versionbump/... \
-cmd "(find /var/lib/rancher/rke2/data/ -type f -name runc -exec {} --version \\;)"  \
-expectedValue "1.1.7"  \
-expectedValueUpgrade "1.10.1" \
-installVersionOrCommit INSTALL_RKE2_VERSION=v1.27.2+rke2r1 \
```

There are also examples on the `Makefile`, which can make things easier by just running the associated `make` command.
