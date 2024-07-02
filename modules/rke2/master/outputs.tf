output "Route53_info" {
  value       = length(aws_route53_record.aws_route53) > 0 ? aws_route53_record.aws_route53[0].fqdn : ""
  description = "List of DNS records"
}

output "kubeconfig" {
  value = "/tmp/${var.resource_name}_kubeconfig"
  description = "kubeconfig of the cluster created"
}

output "master_ips" {
  value = join("," , aws_instance.master.*.public_ip,aws_instance.master2-ha.*.public_ip)
}
output "rendered_template" {
  value = data.template_file.test.rendered
}
