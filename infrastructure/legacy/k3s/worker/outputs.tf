output "Registration_address" {
  value = data.local_file.master_ip.content
}

output "master_node_token" {
  value = data.local_file.token.content
}

output "worker_ips" {
  value = length(aws_eip.worker_with_eip) > 0 ? join(",", aws_eip.worker_with_eip[*].public_ip) : join(",", aws_instance.worker[*].public_ip)
  description = "The public IP of the AWS node"
}

output "worker_flags" {
  value = var.worker_flags
  description = "The worker flags:"
}