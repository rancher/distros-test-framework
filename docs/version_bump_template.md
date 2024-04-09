## Template Bump Version Model

We have a template model interface for testing version bumps. The idea is to provide a simple and direct way to test when a version of a packaged component in either k3s or rke2 is bumped.

The test can be created by adding one version or commit, run some commands on it, and check it against respective expected values then upgrade and repeat the same commands and check the respective new (or not) expected values.


### Tests:
Right now we have 4 tests/jobs that you can run:
 
 CNIs:
`cilium`

`multus + canal` ( looking for calico and flannel also)

 
General components: 
`components` (which runs all those at once )

- Rke2        
```
1- flannel
2- calico
3- ingressController
4- coredns
5- metricsServer
6- etcd
7- containerd
8- runc
```
- k3s
```
1- flannel
2- coredns
3- metrics
4- etcd
5- plugins
6- traefik
7- local-path-storage
8- containerd
9- klipper
10- runc
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
- $ -debug true
```

* All non-boolean arguments are comma separated in case you need to send more than 1.

* If you need to separate another command to run as a single here, separate those with " : " as this example:
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
-installVersionOrCommit INSTALL_K3S_VERSION=v1.27.2+k3s1
```

Example of an execution with multiple values on k3s using components tag:

You should use for rke2 on tfvars file:
```bash
worker_flags   = "profile: cis\nsecrets-encryption: true\nselinux: true\ncni:\n- multus\n- canal\n"
```

```bash
go test -timeout=45m -v -tags=components  ./entrypoint/components/... \
-cmd "flannel,coredns,metrics,etcd,plugins,traefik,local,containerd,klipper,runc" \
-expectedValue "v0.23.0,v3.26.3,nginx-1.9.3,v1.10.1,v0.6.3,v3.5.9,1.7.7,1.1.8" \
-expectedValueUpgrade "v0.23.0,v3.26.3,nginx-1.9.3,v1.10.1,v0.6.3,v3.5.9,1.7.7,1.1.8"

or

go test -timeout=45m -v -tags=components  ./entrypoint/components/... \
-cmd "flannel,coredns,metrics,etcd,plugins,traefik,local,containerd,klipper,runc" \
-expectedValue flannel,coredns,metrics,etcd,plugins,traefik,local,containerd,klipper,runc \
-expectedValueUpgrade flannel,coredns,metrics,etcd,plugins,traefik,local,containerd,klipper,runc
```


There are also examples on the `Makefile`, which can make things easier by just running the associated `make` command.

