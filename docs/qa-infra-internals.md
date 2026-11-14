# QA Infra + DTF: how the two projects work together

Reference for anyone changing either side. It explains how
[`rancher/qa-infra-automation`](https://github.com/rancher/qa-infra-automation) ("qa-infra")
builds a K3s/RKE2 cluster, and how `distros-test-framework` ("DTF") drives it and then
runs its validations on that cluster.
For the operator-facing env vars see [qa-infra-integration.md](qa-infra-integration.md).

## 1. Who owns what

| Concern | Project | Where |
|---|---|---|
| AWS instances, Route53, NLB, bastion | qa-infra | `tofu/aws/modules/{cluster_nodes,dualstack,ipv6,...}` |
| Node prep, product install, cluster join, kubeconfig | qa-infra | `ansible/<product>/<env>/*-playbook*.yml`, `ansible/roles/*` |
| Picking the module/playbook, writing tfvars, inventory, extra-vars | DTF | `internal/provisioning/qainfra/` |
| Run identity, state dir, destroy | DTF | `internal/provisioning/qainfra/runstate.go` |
| Test validations (nodes, pods, ingress, dual-stack, upgrades, ...) | DTF | `entrypoint/<suite>`, `internal/pkg/testcase`, `internal/pkg/assert` |

The legacy path (`PROVISIONER_MODULE` unset or `legacy`) uses DTF's own Terraform in
`infrastructure/legacy` and its own install scripts. It never touches qa-infra, and it
behaves exactly as it did before qa-infra support was added.

## 2. qa-infra on its own

qa-infra is OpenTofu plus Ansible, glued together by a `Makefile`:

```
make infra-up  ENV=<env> DISTRO=<rke2|k3s> PROVIDER=aws   # tofu apply in tofu/<provider>/modules/<env-module>
                                                         # then scripts/generate_inventory.py turns the
                                                         # tofu output cluster_nodes_json into inventory.yml
make cluster   ...                                       # ansible-playbook <playbook for ENV> -i inventory.yml
make all       ...                                       # infra-up + cluster (+ registry/rancher for envs that have them)
```

- **Tofu modules** (`tofu/aws/modules/`). `cluster_nodes` covers default, airgap and proxy.
  PR #184 adds `dualstack` and `ipv6`. These are standalone copies with the same `nodes`
  topology input plus `enable_ipv6`, `enable_public_ip`, `no_of_bastion_nodes`,
  `aws_bastion_subnet`, `kube_api_host_ipv6` and `ipv6_dns64_resolvers`. Every module
  declares its own `aws` provider, so it can also be used as a child module.
- **`cluster_nodes_json` output.** This is the contract between tofu and Ansible:
  `metadata{kube_api_host, fqdn, ssh_user, ...}` plus `nodes[{name, roles, public_ip, private_ip}]`.
  The first etcd node (or the first cp node if there's no etcd) is renamed `master`.
  The dualstack/ipv6 modules add `nodes[].ipv6` and `metadata.bastion_ip/bastion_dns`.
- **Inventory schema** (`ansible/_inventory-schema.yaml`): maps a distro/env to the
  groups `master`, `servers`, `workers` and to the IP field Ansible connects over
  (`public_ip` for dualstack, `ipv6` for ipv6).
- **Playbooks.** `ansible/rke2/default/rke2-playbook.yml`,
  `ansible/rke2/dualstack/rke2-playbook-dualstack.yml`, `ansible/rke2/ipv6/rke2-playbook-ipv6.yml`
  and `ansible/airgap/airgap-playbook.yml`. All of them read `vars.yaml` from the playbook
  dir and write `kubeconfig.yaml` next to it. The dualstack/ipv6 playbooks also run
  `bastion_prepare` (kubectl, docker, podman) on the `bastion` group and copy the
  kubeconfig to `~/.kube/config` on the bastion.
- **IPv6 handling in roles.** `rke2_config` brackets IPv6 literals in `server:` join URLs.
  The playbooks bracket the kubeconfig server address. Inventories built for IPv6 use
  `ssh -W "[%h]:%p"` through the bastion.
- **k3s**: there is no `ansible/k3s/{dualstack,ipv6}` yet, and the Makefile rejects that combination.

## 3. DTF driving qa-infra

### Entry

Every suite's `TestMain` calls `config.AddEnv()`, then `entrypoint.SetupClusterInfra(cfg)`.
That builds a `driver.InfraConfig`, and `provisioning.ProvisionInfrastructure` picks
`legacy` or `qainfra` from `PROVISIONER_MODULE`. `ENV_MODULE` (stored in `InfraConfig.Module`)
selects the qa-infra mode:

| `ENV_MODULE` | qa-infra tofu module | qa-infra ansible dir / playbook |
|---|---|---|
| empty (default) | `cluster_nodes` | `ansible/<product>/default/<product>-playbook.yml` |
| `airgap` | `cluster_nodes` with `airgap_setup=true` and a bastion | `ansible/airgap/airgap-playbook.yml` |
| `dualstack` | `dualstack` | `ansible/rke2/dualstack/rke2-playbook-dualstack.yml` |
| `ipv6only` | `ipv6` | `ansible/rke2/ipv6/rke2-playbook-ipv6.yml` |

The airgap, ipv6only and dualstack suites set `cfg.Module` themselves. The dualstack suite
does it only on qainfra, because on legacy an empty module means the product's own Terraform dir.

### Pipeline (`internal/provisioning/qainfra/qainfra.go`)

1. `loadQAInfra` (config.go): resolves `QA_INFRA_REPO` and `QA_INFRA_REF` to one commit SHA
   (ref.go), so tofu and Ansible run the same revision. It also picks the Ansible dir,
   derives the SSH public key and creates the run id (`QA_INFRA_RUN_ID`).
2. `setupDirectories` and `writeRunManifest` (runstate.go): `$QA_INFRA_STATE_DIR/<run_id>/`
   with `run.json` and, later, `destroy.sh`. The manifest exists before any cloud resource,
   so destroy always finds exactly this run.
3. `prepareTerraformFiles` (opentofu.go): copies `infrastructure/qainfra/{main.tf,variables.tf,vars.tfvars}`,
   then:
   - replaces `placeholder-for-remote-module` with
     `<repo>//tofu/aws/modules/<module>?ref=<sha>`.
   - fills the `# __AIRGAP_MODULE_ARGS__` marker with the airgap args, or with the
     dualstack/ipv6 args (`enable_ipv6`, `enable_public_ip`, `no_of_bastion_nodes=1`,
     `aws_bastion_subnet`). For dualstack/ipv6 it also removes `airgap_setup`/`proxy_setup`,
     which those modules don't declare.
   - writes the `aws_hostname_prefix`, the public key, `create_eip` and the env overrides
     (AMI, user, instance type, region, external DB) into `vars.tfvars`.
4. `executeInfraProvisioner`: `tofu init`, `workspace select -or-create <run_id>`, then
   `apply -var-file=vars.tfvars -var nodes=<from NO_OF_* / split-role env>`. The apply args
   are saved to the manifest first, so `destroy.sh` replays them.
5. `buildClusterConfig`: reads `cluster_nodes_json`. etcd/cp nodes go to `ServerIPs` and the
   rest to `AgentIPs`, using the address tests reach nodes on: public IPv4 by default and
   for dualstack, private for airgap, IPv6 for ipv6only. It also sets `Cluster.Bastion`.
6. `setupAnsibleEnvironment` (ansible.go):
   - sparse-checks-out qa-infra at the same SHA.
   - appends `node-external-ip` to `rke2_config/templates/config.yaml.j2`. The value is
     `{{ ansible_host }}` by default, `{{ ansible_host }},{{ ansible_host_ipv6 }}` for
     dualstack (IPv6 first when `SERVER_FLAGS`' `cluster-cidr` starts with IPv6, because the
     apiserver advertises the first address and it must match the primary service-cidr
     family) and `{{ ansible_host_ipv6 }}` for ipv6. The dual-stack NodePort test curls
     every external IP.
   - installs `cloud.terraform` and exports the `TF_*`/`ANSIBLE_*` env.
   - writes a **static** `inventory.yml` (inventory.go). The dualstack/ipv6 inventory
     (network.go) adds `ansible_host_ipv6` and a `bastion` group. IPv6-only nodes get a
     per-host bracketed ProxyCommand through the bastion; dual-stack nodes are reached
     directly over IPv4.
7. `executeAnsiblePlaybook`: writes `vars.yaml` (version, kubeconfig, cni) and runs the
   playbook with `kubernetes_version`, `server_flags`, `worker_flags`, `channel`,
   `install_method`, `cni` and `rke2_additional_config` (the parsed `SERVER_FLAGS`, which
   wins over the playbook's own `server_flags`) plus `optional_files`. Dualstack/ipv6 runs
   skip `rke2_additional_config` (as #360 does for every run), so the playbook applies
   `server_flags` to servers and `worker_flags` to agents, each parsed as full YAML.
8. `stageBastionKubeconfig` (dualstack/ipv6 only): `cp ~/.kube/config /tmp/rke2_kubeconf.yaml`
   on the bastion. That's the path `GetNodesViaBastion`, `GetPodsViaBastion` and
   `LogClusterInfoUsingBastion` read.
9. `addTofuOutputsToConfig` and `collectAirgapFacts`: record the FQDN and kube_api_host,
   plus the airgap facts.

`resources.KubeConfigFile` points at `<run_dir>/ansible/<ansible dir>/kubeconfig.yaml`.
For dualstack its server is the first cp node's public IPv4, so local `kubectl` works.
For ipv6 it's an IPv6 literal, so the suite uses the `*UsingBastion` helpers instead.

### Destroy

`entrypoint.AfterSuite` calls `provisioning.DestroyInfrastructure`. For qainfra that reads
`run.json` via `QA_INFRA_RUN_ID` and runs `tofu destroy` with the recorded args. It never
matches resources by name. Jenkins also runs `<run_dir>/destroy.sh` in `finally`.

## 4. Example: validatecluster (default mode)

```bash
# config/.env (see qa-infra-integration.md for the full list)
PROVISIONER_MODULE=qainfra
QA_INFRA_PROVIDER=aws
QA_INFRA_REF=main
ENV_PRODUCT=rke2
INSTALL_VERSION=v1.35.0+rke2r1
RESOURCE_NAME=me01
SSH_USER=ubuntu
SSH_LOCAL_KEY_PATH=/path/to/aws_key.pem
NODE_OS=ubuntu
NO_OF_SERVER_NODES=1
NO_OF_WORKER_NODES=1
TEST_DIR=validatecluster
# infrastructure/qainfra/vars.tfvars holds region/vpc/subnet/sg/ami/zone
make test-run        # or: go test -timeout=60m -v -count=1 ./entrypoint/validatecluster/... -destroy=true
```

## 5. Dual-stack and IPv6-only on qainfra

Requires a qa-infra ref with PR #184 (rancher/qa-infra-automation#184).
Only `ENV_PRODUCT=rke2` and `QA_INFRA_PROVIDER=aws` are accepted; anything else fails
before any resource is created. The `dualstack`/`ipv6` modules need OpenTofu >= 1.9.

`vars.tfvars` must point at a **dual-stack** VPC, subnet and security group. Optionally set `aws_bastion_subnet`, which defaults to `aws_subnet`.
Don't put `enable_ipv6`, `enable_public_ip` or `no_of_bastion_nodes` in tfvars: DTF sets
them for the mode. For IPv6-first dual-stack, set `kube_api_host_ipv6 = true` in tfvars so
nodes join over the master's IPv6 (ipv6only always uses IPv6).

```bash
# dual-stack
TEST_DIR=dualstack
SERVER_FLAGS="cluster-cidr: 10.42.0.0/16,2001:cafe:42::/56\nservice-cidr: 10.43.0.0/16,2001:cafe:43::/112"
WORKER_FLAGS="kubelet-arg:\n  - node-ip=0.0.0.0"
# SERVER_FLAGS go to etcd/cp nodes and WORKER_FLAGS to agents, parsed as YAML (lists work).
# Per-node templates work too: node-ip: {{ ansible_host_ipv6 }},{{ ansible_default_ipv4.address }}

# IPv6 only (ENV_MODULE=ipv6only is set by the suite)
TEST_DIR=ipv6only
SERVER_FLAGS="cluster-cidr: 2001:cafe:42::/56\nservice-cidr: 2001:cafe:43::/112"
```

What the suites do on qainfra:

- `dualstack`: same specs as legacy. Ingress and NodePort are curled from the bastion
  over both families; cluster IPs are checked against the `cluster-cidr`/`service-cidr`
  read from the server node's args.
- `ipv6only`: the legacy "Install product on ipv6 only nodes" spec is **skipped**, because
  Ansible already installed RKE2. Node and pod checks run through the bastion against
  `/tmp/rke2_kubeconf.yaml`.

IPv6-only nodes resolve IPv4-only endpoints (`get.rke2.io`, image registries) through
the module's DNS64 default (`ipv6_dns64_resolvers`, the public nat64.net service).
