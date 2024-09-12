## Template Bump Version Model

We have a template model interface for testing version bumps. The idea is to provide a simple and direct way to test when a version of a packaged component in either k3s or rke2 is bumped.

The test can be created by adding one version or commit, run some commands on it, and check it against respective expected values then upgrade and repeat the same commands and check the respective new (or not) expected values.


### Tests:
Right now we have 4 tests/jobs that you can run:
 
 CNIs:
`cilium`

`multus + canal` 

`flannel`
 
General components: 
`components` (which runs all those at once )

- Rke2        
```
1- kubernetes
2- coredns
3- metrics server
4- etcd
5- containerd
6- runc
7- crictl
8- canalFlannel
9- calico
10- ingressController

```
- k3s
```
1- kubernetes
2- coredns
3- metrics server
4- etcd
5- cni plugins
6- containerd
7- runc
8- crictl
9- traefik
10- local path provisioner
11- klipper LB
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

