output "master_ips" {
  value       = module.master.master_ips
  description = "The public IP of the server node in AWS"
}

output "worker_ips" {
  value       = module.worker.worker_ips
  description = "The public IP of the agent node in AWS"
}

output "windows_worker_ips" {
  value       = module.windows_worker.windows_worker_ips
  description = "The public IP of the windows agent node in AWS"
}

output "kubeconfig" {
  value = module.master.kubeconfig
  description = "kubeconfig of the cluster created"
}

output "Route53_info" {
  value = module.master.Route53_info
  description = "List of DNS records"
}

output "bastion_ip" {
  depends_on = [ module.bastion ]
  value       = module.bastion.public_ip
  description = "The public IP of the bastion node"
}

output "bastion_dns" {
  depends_on = [ module.bastion ]
  value       = module.bastion.public_dns
  description = "The public DNS of the bastion node"
}

output "rendered_template" {
  value = module.master.rendered_template
}

