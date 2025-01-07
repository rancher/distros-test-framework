output "public_ip" {
  value = var.no_of_bastion_nodes > 0 ? aws_instance.bastion[0].public_ip : ""
  description = "The public IP of the AWS node"
}

output "public_dns" {
  value = var.no_of_bastion_nodes > 0 ? aws_instance.bastion[0].public_ip : ""
  description = "The public DNS of the AWS node"
}

output "master_ips" {
  value = join("," ,aws_instance.master.*.public_ip)
  description = "The public IP of the AWS master node"
}

output "worker_ips" {
  value = (var.no_of_worker_nodes > 0 && var.enable_public_ip) ? join("," ,aws_instance.worker.*.public_ip) : ""
  description = "The public IP of the AWS worker node"
}

output "master_ipv6" {
  value = join("," ,aws_instance.master.*.ipv6_addresses[0])
  description = "The IPv6 IP of the AWS master node"
}

output "worker_ipv6" {
  value = (var.no_of_worker_nodes > 0 && var.enable_ipv6) ? join("," ,aws_instance.worker.*.ipv6_addresses[0]) : ""
  description = "The IPv6 IP of the AWS worker node"
}
