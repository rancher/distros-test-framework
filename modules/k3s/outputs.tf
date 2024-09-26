output "master_ips" {
  value       = module.master.master_ips
  description = "The public IP of the AWS node"
}

output "worker_ips" {
  value       = module.worker.worker_ips
  description = "The public IP of the AWS node"
}

output "kubeconfig" {
  value = module.master.kubeconfig
  description = "kubeconfig of the cluster created"
}

output "rendered_template" {
  value = module.master.rendered_template
}

output "server_flags" {
  value = var.server_flags
  description = "The server flags:"
}

output "worker_flags" {
  value = var.worker_flags
  description = "The worker flags:"
}

output "Route53_info" {
  value = module.master.Route53_info
  description = "List of DNS records"
}

output "bastion_ip" {
  value       = module.bastion.public_ip
  description = "The public IP of the bastion node"
}

output "bastion_dns" {
  value       = module.bastion.public_dns
  description = "The public DNS of the bastion node"
}