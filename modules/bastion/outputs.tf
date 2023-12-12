output "public_ip" {
  value = join("," , aws_instance.bastion.*.public_ip)
  description = "The public IP of the AWS node"
}

output "id" {
  value = join("," , aws_instance.bastion.*.id)
  description = "The ID of the AWS bastion node"
}