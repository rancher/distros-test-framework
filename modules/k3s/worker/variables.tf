variable "dependency" {
  type    = any
  default = null
}
variable "region" {}
variable "aws_ami" {}
variable "aws_user" {}
variable "vpc_id" {}
variable "subnets" {}
variable "resource_name" {}
variable "access_key" {}
variable "k3s_version" {}
variable "no_of_worker_nodes" {
  default = 0
}
variable "worker_flags" {}
variable "ec2_instance_class" {}
variable "availability_zone" {}
variable "sg_id" {}
variable  "volume_size" {}
variable "rhel_username" {
  default = "rhel_username"
}
variable "rhel_password" {
  default = "rhel_password"
}
variable "node_os" {}
variable "install_mode" {}
variable "key_name" {}
variable "k3s_channel" {
  default = "testing"
}
variable "enable_public_ip" {}
variable "enable_ipv6" {
  default = false
}