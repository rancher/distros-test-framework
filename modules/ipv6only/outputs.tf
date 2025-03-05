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
  description = "The IPv6 IP of the AWS node"
}

output "worker_ips" {
  value       = module.instance.worker_ips
  description = "The IPv6 IP of the AWS node"
}
