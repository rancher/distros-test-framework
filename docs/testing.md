### Windows Testing

- Required vars for `rke2.tfvars` file
```
# Windows AWS User is Administrator by default.
# It is also ideal to allocate higher resource for Windows agent.

server_flags               = "cni: calico\n"
windows_ec2_instance_class = "t3.xlarge"
windows_aws_ami            = "ami-05a418fd6eb36fd5b"
no_of_windows_worker_nodes = 1
split_roles                = false
```
- Test package should be `mixedoscluster`
- Split roles is not supported at this time (Future enhancement)
- Hardened setup is not supported with Windows
- CNI should be calico


### Dual-Stack Testing

- Required vars for `*.tfvars` file
- `kubelet-arg: \n - node-ip=0.0.0.0` is needed if the public and private IPs are same

```
enable_public_ip   = true
enable_ipv6        = true
server_flags       = "cluster-cidr: <ipv4-cluster-cidr>,<ipv6-cluster-cider>\nservice-cidr: <ipv4-service-cidr>,<ipv6-service-cidr>\nkubelet-arg: \n - node-ip=0.0.0.0\n"
no_of_bastion_nodes = 1
bastion_subnets     = "<dual-stack-subnet>"

```
- Test package should be `dualstack`
- Split roles is not supported at this time (Future enhancement)
- Reorder IP is not supported at this time (Future enhancement)
