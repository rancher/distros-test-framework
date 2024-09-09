output "bastion_ip" {
  depends_on = [ aws_instance.bastion ]
  value = aws_instance.bastion[0].public_ip
  description = "The public IP of the AWS bastion node"
}

output "bastion_dns" {
  depends_on = [ aws_instance.bastion ]
  value = aws_instance.bastion[0].public_dns
  description = "The public DNS of the AWS bastion node"
}

output "master_ips" {
  depends_on = [ aws_instance.master ]
  value = var.enable_ipv6 ? join("," ,aws_instance.master.*.ipv6_addresses[0]) : join("," ,aws_instance.master.*.private_ip)
  description = "The private IP or IPv6 IP of the AWS private master node"
}

output "worker_ips" {
  depends_on = [ aws_instance.worker ]
  value = var.enable_ipv6 && var.no_of_worker_nodes >=1 ? join("," ,aws_instance.worker.*.ipv6_addresses[0]) : join("," ,aws_instance.worker.*.private_ip)
  description = "The private IP or IPv6 IP of the AWS private worker node"
}

# output "master_hosts" {
#   value = join("," ,aws_instance.master.*.private_dns)
#   description = "The hostname of the AWS private master node"
#   depends_on = [ aws_instance.master ]
# }

# output "worker_hosts" {
#   value =  join("," ,aws_instance.worker.*.private_dns)
#   description = "The hostname of the AWS private worker node"
#   depends_on = [ aws_instance.worker ]
# }

# output "windows_agent_ips" {
#   value = join("," , aws_instance.windows_agent.private_ip, aws_instance.windows_agent.ipv6_addresses[0])
#   description = "The private IP and/or IPV6 IP of the AWS node"
# }

output "check_airgap" {
  value = data.template_file.is_airgap
}

output "check_ipv6only" {
  value = data.template_file.is_ipv6only
}
