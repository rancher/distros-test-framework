output "Route53_info" {
  value       = aws_route53_record.aws_route53.*
  description = "List of DNS records"
}

output "server_ips" {
  value = join("," , aws_instance.server.*.public_ip,aws_instance.master2-ha.*.public_ip)
  description = "The public IP of the AWS node"
}

output "kubeconfig" {
  value = "/tmp/${var.resource_name}_kubeconfig"
  description = "kubeconfig of the cluster created"
}

output "rendered_template" {
  value = data.template_file.test.rendered
}

output "server_flags" {
  value = var.server_flags
  description = "The server flags:"
}