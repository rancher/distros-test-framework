output "Route53_info" {
  value       = length(aws_route53_record.aws_route53) > 0 ? aws_route53_record.aws_route53[0].fqdn : ""
  description = "List of DNS records"
}

output "kubeconfig" {
  value = "/tmp/${var.resource_name}_kubeconfig"
  description = "kubeconfig of the cluster created"
}

output "master_ips" {
  value = var.create_eip ? join("," , aws_eip.master_with_eip.*.public_ip,aws_eip.master2_with_eip.*.public_ip): join("," , aws_instance.master.*.public_ip,aws_instance.master2-ha.*.public_ip)
  description = "The public IP of the AWS node"
}

output "rendered_template" {
  value = data.template_file.test.rendered
}

output "eip_public_ip" {
  value = var.create_eip ? aws_eip.master_with_eip[0].public_ip : null
}