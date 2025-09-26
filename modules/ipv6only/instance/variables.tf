variable "dependency" {
  type    = any
  default = null
}
variable "aws_ami" {}
variable "aws_user" {}
variable "region" {}
variable "vpc_id" {}
variable "subnets" {}
variable "bastion_subnets" {}
variable "availability_zone" {}
variable "sg_id" {}
variable "ec2_instance_class" {}
variable "resource_name" {}
variable "volume_size" {}
variable "key_name" {}
variable "access_key" {}
variable "no_of_bastion_nodes" {}
variable "enable_public_ip" {
  default = false
}
variable "enable_ipv6" {
  default = true
}
variable "no_of_worker_nodes" {}
variable "no_of_server_nodes" {}
variable "product" {}
variable "create_lb" {
  default = false
}
variable "hosted_zone" {}
