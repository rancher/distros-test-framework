output "public_ip" {
  value = var.no_of_bastion_nodes != 0 ? aws_instance.bastion[0].public_ip : ""
  description = "The public IP of the AWS node"
  
}

output "public_dns" {
  value = var.no_of_bastion_nodes != 0 ? aws_instance.bastion[0].public_ip : ""
  description = "The public DNS of the AWS node"
}
