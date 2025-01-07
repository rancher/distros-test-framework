output "bastion_ip" {
  depends_on = [ module.instance ]
  value       = module.instance.public_ip
  description = "The public IP of the bastion node"
}

output "bastion_dns" {
  depends_on = [ module.instance ]
  value       = module.instance.public_dns
  description = "The public DNS of the bastion node"
}

output "master_ips" {
  value       = module.instance.master_ips
  description = "The private IP of the AWS node"
}

output "worker_ips" {
  value       = module.instance.worker_ips
  description = "The private IP of the AWS node"
}

output "master_ipv6" {
  value       = module.instance.master_ipv6
  description = "The IPv6 IP of the AWS node"
}

output "worker_ipv6" {
  value       = module.instance.worker_ipv6
  description = "The IPv6 IP of the AWS node"
}