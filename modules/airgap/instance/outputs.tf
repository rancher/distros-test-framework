output "bastion_ip" {
  value = aws_instance.bastion[0].public_ip
  description = "The public IP of the AWS bastion node"
}

output "bastion_dns" {
  value = aws_instance.bastion[0].public_dns
  description = "The public DNS of the AWS bastion node"
}

output "master_ips" {
  value = var.enable_ipv6 ? join("," ,aws_instance.master.*.ipv6_addresses[0]) : join("," ,aws_instance.master.*.private_ip)
  description = "The private IP or IPv6 IP of the AWS private master node"
}

output "worker_ips" {
  value = var.enable_ipv6 ? join("," ,aws_instance.worker.*.ipv6_addresses[0]) : join("," ,aws_instance.worker.*.private_ip)
  description = "The private IP or IPv6 IP of the AWS private worker node"
}

output "check_airgap" {
  value = data.template_file.is_airgap
}

output "check_ipv6only" {
  value = data.template_file.is_ipv6only
}
