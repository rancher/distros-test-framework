## Template Bump Version Model

We have a template model interface for testing version bumps. The idea is to provide a simple and direct way to test when a version of a packaged component in either k3s or rke2 is bumped.

The test can be created by adding one version or commit, run some commands on it, and check it against respective expected values then upgrade and repeat the same commands and check the respective new (or not) expected values.


### Tests:
We have 5 tests/jobs that you can run for:
 
 CNIs:
`cilium`

`multus + canal` 

`flannel`

`calico`

`canal`

And we have 2 jobs that you can run for:

General components: 
`components` (which runs all except the cni's at once )

`versionbump` (which takes cmds and expected values along with upgrades)

- In case of adding new components, you should be updating the values lenght at:
https://github.com/rancher/distros-test-framework/blob/bfe96fc37b42eff755b2f800f912bc4f78f91972/pkg/customflag/validate.go#L143

- Rke2        
```
1- coredns
2- metrics server
3- etcd
4- containerd
5- runc
6- crictl
7- ingressController

```
- k3s
```
1- coredns
2- metrics server
3- etcd
4- cni plugins
5- containerd
6- runc
7- crictl
8- traefik
9- local path provisioner
10- klipper LB
```

Version bump: 
- Runs any combination of cmd x expected value you want.

### How can I do that?

Available flags to create your tests with some data examples:
```
- $ -cmd "kubectl describe pod -n kube-system local-path-provisioner- : | grep -i Image"
- $ -expectedValue "v0.0.21"
- $ -expectedValueUpgrade "v0.0.24"
- $ -installVersionOrCommit 257fa2c54cda332e42b8aae248c152f4d1898218
- $ -applyWorkload true
- $ -deleteWorkload true
- $ -testCase "TestLocalPathProvisionerStorage"
- $ -workloadName "bandwidth-annotations.yaml"
- $ -description "Description of your test"
```

* All non-boolean arguments are comma separated in case you need to send more than 1.

* If you need to provide multiple commands as a single command, use the colon separator ":" to separate those commands as shown in the example below:
The shell command separators can also be used within the commands like ";" , "|" , "&&" etc.
-cmd "kubectl describe pod -n kube-system local-path-provisioner- :  | grep -i Image"


* All flags are optional except for -cmd and -expectedValue but if you want to use one of them, you need to use all of them.


Example of an execution with multiple values on k3s using versionbump tag:
```bash
go test -timeout=45m -v -tags=versionbump  ./entrypoint/versionbump/... \
-cmd "/var/lib/rancher/k3s/data/current/bin/cni, kubectl get pod test-pod -o yaml : | grep -A2 annotations, k3s -v" \
-expectedValue "v1.2.0-k3s1,1M, v1.26" \
-expectedValueUpgrade "v1.2.0-k3s1,1M, v1.27" \
-installVersionOrCommit v1.27.2+k3s1 \
-testCase "TestServiceClusterIP, TestLocalPathProvisionerStorage" \
-applyWorkload=true \
-deleteWorkload=false \
-workloadName "bandwidth-annotations.yaml"
```

Example of an execution with less args on k3s using versionbump tag:
```bash
go test -timeout=45m -v -tags=versionbump  ./entrypoint/versionbump/... \
-cmd "/var/lib/rancher/k3s/data/current/bin/cni, kubectl get pod test-pod -o yaml : | grep -A2 annotations, k3s -v"  \
-expectedValue "v1.2.0-k3s1,1M, v1.26"  \
-expectedValueUpgrade "v1.2.0-k3s1,1M, v1.27" \
-installVersionOrCommit v1.27.2+k3s1
```


There are also examples on the `Makefile`, which can make things easier by just running the associated `make` command.

