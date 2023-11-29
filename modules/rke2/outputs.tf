output "server_ips" {
  value       = module.server.server_ips
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
  value = module.server.kubeconfig
  description = "kubeconfig of the cluster created"
}