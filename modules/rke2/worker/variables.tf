variable "access_key" {}
variable "key_name" {}
variable "availability_zone" {}
variable "aws_ami" {}
variable "aws_user" {}
variable "dependency" {
  type    = any
  default = null
}
variable "ec2_instance_class" {}
variable "volume_size" {}
variable "iam_role" {}
variable "node_os" {}
variable "no_of_worker_nodes" {}
variable "region" {}
variable "resource_name" {}
variable "rke2_version" {}
variable "install_mode" {}
variable "install_method" {}
variable "rke2_channel" {}
variable "sg_id" {}
variable "subnets" {}
variable "vpc_id" {}
variable "enable_public_ip" {}
variable "enable_ipv6" {}
variable "worker_flags" {}
variable "rhel_username" {
  default = "rhel_username"
}
variable "rhel_password" {
  default = "rhel_password"
}