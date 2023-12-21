output "public_ip" {
  value = join("," ,aws_instance.bastion.*.public_ip)
  description = "The public IP of the AWS node"
}

output "server_ips" {
  value = join("," ,aws_instance.server.*.private_ip)
  description = "The private IP and/or IPV6 IP of the AWS node"
}

output "agent_ips" {
  value = join("," ,aws_instance.agent.*.private_ip)
  description = "The private IP and/or IPV6 IP of the AWS node"
}

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
