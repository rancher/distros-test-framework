output "public_ip" {
  value = var.no_of_bastion_nodes == 1 ? aws_instance.bastion[0].public_ip : ""
  description = "The public IP of the AWS node"
  depends_on = [ aws_instance.bastion ]
}

# output "id" {
#   value = var.no_of_bastion_nodes == 1 ? aws_instance.bastion[0].id : ""
#   description = "The ID of the AWS bastion node"
# }