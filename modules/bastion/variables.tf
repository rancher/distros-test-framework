variable "dependency" {
  type    = any
  default = null
}
variable "aws_ami" {}
variable "aws_user" {}
variable "region" {}
variable "vpc_id" {}
variable "bastion_subnets" {
  default = ""
}
variable "availability_zone" {}
variable "sg_id" {}
variable "ec2_instance_class" {}
variable "resource_name" {}
variable "key_name" {}
variable "access_key" {}
variable "no_of_bastion_nodes" {
  default = 0
}
variable "enable_public_ip" {
  default = true
}
variable "enable_ipv6" {
  default = false
}


