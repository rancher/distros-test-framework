 terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 3.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.1"
    }
    ansible = {
      source  = "ansible/ansible"
      version = "1.3.0"
    }
    vsphere = {
		source  = "hashicorp/vsphere"
		version = "~> 2.0"
	  }
  }
}

module "cluster_nodes" {
  source = "placeholder-for-remote-module"
  user_id            = var.user_id
  public_ssh_key     = var.public_ssh_key
  aws_access_key     = var.aws_access_key
  aws_secret_key     = var.aws_secret_key
  aws_region         = var.aws_region
  aws_ami            = var.aws_ami
  instance_type      = var.instance_type
  aws_security_group = var.aws_security_group
  aws_subnet         = var.aws_subnet
  aws_vpc            = var.aws_vpc
  aws_route53_zone   = var.aws_route53_zone
  aws_ssh_user       = var.aws_ssh_user
  aws_volume_size    = var.aws_volume_size
  aws_volume_type    = var.aws_volume_type
  aws_hostname_prefix = var.aws_hostname_prefix
  airgap_setup       = var.airgap_setup
  proxy_setup        = var.proxy_setup
  nodes              = var.nodes
}

output "fqdn" {
  value = module.cluster_nodes.fqdn
}

output "kube_api_host" {
  value = module.cluster_nodes.kube_api_host
}
