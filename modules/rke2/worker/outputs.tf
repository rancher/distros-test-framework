output "worker_ips" {
  value = var.create_eip ? join(",", aws_eip.worker_with_eip[*].public_ip) : join(",", aws_instance.worker[*].public_ip)
  description = "The public IP of the AWS node"
}
