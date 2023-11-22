output "Route53_info" {
  value       = aws_route53_record.aws_route53.*
  description = "List of DNS records"
}

output "kubeconfig" {
  value = "/tmp/${var.resource_name}_kubeconfig"
  description = "kubeconfig of the cluster created"
}

output "server_ips" {
  value = join("," , aws_instance.server.*.public_ip,aws_instance.server2.*.public_ip)
}