variable "no_of_worker_nodes" {}
variable "aws_ami" {}
variable "aws_user" {}
variable "region" {}
variable "access_key" {}
variable "vpc_id" {}
variable "subnets" {}
variable "qa_space" {}
variable "resource_name" {}
variable "key_name" {}
variable "external_db" {}
variable "external_db_version" {}
variable "db_instance_class" {}
variable "ec2_instance_class" {}
variable "db_group_name" {}
variable "rhel_username" {
  default = "rhel_username"
}
variable "rhel_password" {
  default = "rhel_password"
}
variable "k3s_version" {}
variable "no_of_server_nodes" {}
variable "server_flags" {}
variable "worker_flags" {}
variable "availability_zone" {}
variable "sg_id" {}
variable "volume_size" {}
variable "enable_public_ip" {
  default = true
}
variable "enable_ipv6" {
  default = false
}
variable "datastore_type" {}
variable "node_os" {}
variable "db_username" {}
variable "db_password" {}
variable "db_environment" {}
variable "engine_mode" {}
variable "arch" {}
variable "install_mode" {}
variable "k3s_channel" {
  default = "testing"
}
variable "create_lb" {
  description = "Create Network Load Balancer if set to true"
  type = bool
}
variable "no_of_bastion_nodes" {
  default = 0
}
variable "bastion_subnets" {
  default = ""
}
variable "bastion_id" {
  type    = any
  default = null
}
