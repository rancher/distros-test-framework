output "Registration_address" {
  value = data.local_file.server_ip.content
}

output "server_node_token" {
  value = data.local_file.token.content
}

output "worker_ips" {
  value = join("," , aws_instance.worker.*.public_ip)
  description = "The public IP of the AWS node"
}

output "worker_flags" {
  value = var.worker_flags
  description = "The worker flags:"
}