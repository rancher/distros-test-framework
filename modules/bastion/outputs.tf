output "public_ip" {
  value = join("," , aws_instance.bastion.*.public_ip)
  description = "The public IP of the AWS node"
}
