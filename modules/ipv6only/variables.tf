
  # AWS variables
variable "access_key" {}
variable "key_name" {}
variable "availability_zone" {}
variable "aws_ami" {}
variable "aws_user" {}
variable "ec2_instance_class" {}
variable "iam_role" {}
variable "region" {}
variable "resource_name" {}
variable "sg_id" {}
variable "subnets" {}
variable "vpc_id" {}
variable "volume_size" {}
variable "enable_public_ip" {
  default = true
}
variable "enable_ipv6" {
  default = false
}
variable "no_of_bastion_nodes" {
  default = 0
}
variable "no_of_server_nodes" {}
variable "no_of_worker_nodes" {}
variable "bastion_subnets" {
  default = ""
}
variable "bastion_id" {
  type    = any
  default = null
}
variable "arch" {
  default = "amd64"
}
variable "product" {}
variable "etcd_only_nodes" {}
variable "etcd_cp_nodes" {}
variable "etcd_worker_nodes" {}
variable "cp_only_nodes" {}
variable "cp_worker_nodes" {}
