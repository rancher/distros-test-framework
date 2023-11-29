output "server_ips" {
  value       = module.server.server_ips
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