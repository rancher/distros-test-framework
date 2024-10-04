output "master_ips" {
  value       = module.instance.master_ips
  description = "The private IP of the AWS node"
}

output "worker_ips" {
  value       = module.instance.worker_ips
  description = "The private IP of the AWS node"
}

output "bastion_ip" {
  value       = module.instance.bastion_ip
  description = "The public IP of the AWS node"
}

output "bastion_dns" {
  value       = module.instance.bastion_dns
  description = "The public DNS of the AWS node"
}

output "check_airgap" {
  value = module.instance.check_airgap.rendered
}

output "check_ipv6only" {
  value = module.instance.check_ipv6only.rendered
}
