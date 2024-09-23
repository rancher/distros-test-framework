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
variable "subnets" {
  default = ""
}
variable "availability_zone" {}
variable "sg_id" {}
variable "ec2_instance_class" {}
variable "resource_name" {}
variable "volume_size" {}
variable "key_name" {}
variable "access_key" {}
variable "username" {
  default = "username"
}
variable "password" {
  default = "password"
}
variable "no_of_bastion_nodes" {
  default = 0
}
variable "enable_public_ip" {
  default = true
}
variable "enable_ipv6" {
  default = false
}
variable "product" {}
variable "install_mode" {}
variable "install_version" {}
variable "channel" {}
variable "install_method" {}
variable "no_of_server_nodes" {}
variable "no_of_worker_nodes" {}
variable "arch" {
  default = "amd64"
}

