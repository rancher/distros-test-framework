output "master_ips" {
  value       = module.bastion.server_ips
  description = "The public IP of the AWS node"
}

output "worker_ips" {
  value       = module.bastion.agent_ips
  description = "The public IP of the AWS node"
}

# output "windows_worker_ips" {
#   value       = module.bastion.agent_ips
#   description = "The public IP of the AWS node"
# }

# output "kubeconfig" {
#   value = module.bastion.kubeconfig
#   description = "kubeconfig of the cluster created"
# }

output "bastion_ip" {
  value       = module.bastion.public_ip
  description = "The public IP of the AWS node"
}

output "check_airgap" {
  value = module.bastion.check_airgap.rendered
}

output "check_ipv6only" {
  value = module.bastion.check_ipv6only.rendered
}