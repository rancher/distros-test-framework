terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.1"
    }
    ansible = {
      source  = "ansible/ansible"
      version = "1.3.0"
    }
  }
}

# Root-level AWS provider needed by the data/aws_eip resources below. The
# upstream cluster_nodes module declares its own provider with the same
# inputs, but root-level resources can't see that.
#
# When aws_access_key/aws_secret_key are empty strings (their defaults in
# vars.tfvars), the AWS provider falls back to the standard credentials
# chain — env vars AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY (passed in by
# scripts/docker_run.sh) or ~/.aws. Region must be set explicitly because
# docker_run.sh doesn't forward AWS_REGION.
provider "aws" {
  region     = var.aws_region
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key
}

module "cluster_nodes" {
  source                  = "placeholder-for-remote-module"
  public_ssh_key          = var.public_ssh_key
  aws_access_key          = var.aws_access_key
  aws_secret_key          = var.aws_secret_key
  aws_region              = var.aws_region
  aws_ami                 = var.aws_ami
  instance_type           = var.instance_type
  aws_security_group      = var.aws_security_group
  aws_subnet              = var.aws_subnet
  aws_vpc                 = var.aws_vpc
  aws_route53_zone        = var.aws_route53_zone
  aws_ssh_user            = var.aws_ssh_user
  aws_volume_size         = var.aws_volume_size
  aws_volume_type         = var.aws_volume_type
  aws_hostname_prefix     = var.aws_hostname_prefix
  airgap_setup            = var.airgap_setup
  proxy_setup             = var.proxy_setup
  nodes                   = var.nodes
  aws_ami_windows         = var.aws_ami_windows
  instance_type_windows   = var.instance_type_windows
  aws_volume_size_windows = var.aws_volume_size_windows
  aws_volume_type_windows = var.aws_volume_type_windows
  aws_windows_ssh_user    = var.aws_windows_ssh_user
  windows_enable_rdp      = var.windows_enable_rdp
}

# Airgap-only arguments (bastion, run_id, qa_infra_sha, arch) are injected here by
# opentofu.go so connected runs stay compatible with qa-infra refs that predate them.
# __AIRGAP_MODULE_ARGS__

# external_db module (Path B) is injected here by opentofu.go only when DATASTORE_TYPE=external and no endpoint was supplied.
# __EXTERNAL_DB_MODULE__

locals {
  # Replicate upstream cluster_nodes' name-generation for linux instances.
  temp_node_names = flatten([
    for node_group in var.nodes : [
      for i in range(node_group.count) : {
        name = "${join("-", node_group.role)}-${i}"
        role = node_group.role
      }
    ] if try(node_group.os, "linux") == "linux"
  ])

  # First etcd node becomes primary master; fall back to first cp node.
  first_etcd_index   = try(index([for n in local.temp_node_names : contains(n.role, "etcd")], true), -1)
  first_cp_index     = try(index([for n in local.temp_node_names : contains(n.role, "cp")], true), -1)
  first_master_index = local.first_etcd_index >= 0 ? local.first_etcd_index : local.first_cp_index

  linux_node_names = [
    for idx, n in local.temp_node_names :
    idx == local.first_master_index ? "master" : n.name
  ]

  windows_nodes_unnamed = flatten([
    for group in var.nodes : [
      for i in range(group.count) : { roles = ["worker"] }
    ] if try(group.os, "linux") == "windows"
  ])
  windows_temp_nodes = [
    for i, node in local.windows_nodes_unnamed :
    merge(node, { name = "windows-worker-${i}" })
  ]

  node_names = concat(local.linux_node_names, [for w in local.windows_temp_nodes : w.name])

  primary_name  = local.first_master_index >= 0 ? "master" : ""
  node_name_set = toset(local.node_names)
}

# Look up each instance by its Name tag so we can attach EIPs without
# requiring upstream to expose instance IDs as a separate output.
data "aws_instance" "node" {
  for_each = var.create_eip ? local.node_name_set : toset([])

  filter {
    name   = "tag:Name"
    values = ["tf-${var.aws_hostname_prefix}-${each.value}"]
  }

  depends_on = [module.cluster_nodes]
}

resource "aws_eip" "node" {
  for_each = var.create_eip ? local.node_name_set : toset([])
  domain   = "vpc"

  tags = {
    Name = "tf-${var.aws_hostname_prefix}-${each.value}-eip"
  }
}

resource "aws_eip_association" "node" {
  for_each      = aws_eip.node
  instance_id   = data.aws_instance.node[each.key].id
  allocation_id = each.value.id
}

output "fqdn" {
  value = module.cluster_nodes.fqdn
}

# When create_eip is true, kube_api_host needs to be the primary master's
# EIP — otherwise tooling that reads `tofu output -raw kube_api_host` ends
# up with an ephemeral IP that goes stale at first reboot.
output "kube_api_host" {
  value = (
    var.create_eip && local.primary_name != ""
    ? aws_eip.node[local.primary_name].public_ip
    : module.cluster_nodes.kube_api_host
  )
}

# Pass through cluster_nodes_json for the inventory generator / node extractor; when create_eip is true, rewrite the IPs to reference EIPs.
output "cluster_nodes_json" {
  value = (
    var.create_eip && local.primary_name != ""
    ? jsonencode({
      type = jsondecode(module.cluster_nodes.cluster_nodes_json).type
      metadata = merge(
        jsondecode(module.cluster_nodes.cluster_nodes_json).metadata,
        {
          kube_api_host = aws_eip.node[local.primary_name].public_ip
        },
      )
      nodes = [
        for n in jsondecode(module.cluster_nodes.cluster_nodes_json).nodes :
        merge(n, { public_ip = aws_eip.node[n.name].public_ip })
      ]
      # schema v2 field; null on qa-infra refs that predate it.
      bastion = try(jsondecode(module.cluster_nodes.cluster_nodes_json).bastion, null)
    })
    : module.cluster_nodes.cluster_nodes_json
  )
}

output "instance_public_ips" {
  value = (
    var.create_eip
    ? [for n in local.node_names : aws_eip.node[n].public_ip]
    : module.cluster_nodes.instance_public_ips
  )
}

output "windows_instance_public_ips" {
  value = module.cluster_nodes.windows_instance_public_ips
}

output "windows_administrator_passwords" {
  sensitive = true
  value     = module.cluster_nodes.windows_administrator_passwords
}
