output "master_ips" {
  value       = module.instance.master_ips
  description = "The private IP of the AWS node"
}

output "worker_ips" {
  value       = module.instance.worker_ips
  description = "The private IP of the AWS node"
}

output "windows_worker_ips" {
  value       = module.instance.windows_worker_ips
  description = "The private IP of the AWS Windows node"
}

output "windows_worker_password_decrypted" {
  value       = module.instance.windows_worker_password_decrypted
  description = "The decrypted password of the AWS Windows node"
}

output "bastion_ip" {
  value       = module.instance.bastion_ip
  description = "The public IP of the AWS node"
}

output "bastion_dns" {
  value       = module.instance.bastion_dns
  description = "The public DNS of the AWS node"
}

