output "bastion_public_ip" {
  value = var.no_of_bastion_nodes > 0 ? aws_instance.bastion[0].public_ip : ""
  description = "The public IP of the AWS node"
}

output "bastion_public_dns" {
  value = var.no_of_bastion_nodes > 0 ? aws_instance.bastion[0].public_dns : ""
  description = "The public DNS of the AWS node"
}

output "master_ips" {
  value = join(",", [
    for instance in aws_instance.master : instance.ipv6_addresses[0]
  ])
  description = "The IPv6 IP of the AWS master node"
}

output "worker_ips" {
  value = join("," , [
    for instance in aws_instance.worker : instance.ipv6_addresses[0]
  ])
  description = "The IPv6 IP of the AWS worker node"
}
