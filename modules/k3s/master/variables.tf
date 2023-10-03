variable "aws_ami" {}
variable "aws_user" {}
variable "region" {}
variable "access_key" {}
variable "vpc_id" {}
variable "subnets" {}
variable "availability_zone" {}
variable "sg_id" {}
variable "volume_size" {}
variable "qa_space" {}
variable "ec2_instance_class" {}
variable "resource_name" {}
variable "key_name" {}

variable "username" {
  default = "username"
}
variable "password" {
  default = "password"
}
variable "k3s_version" {}
variable "no_of_server_nodes" {}
variable "server_flags" {}
variable "enable_public_ip" {}
variable "enable_ipv6" {
  default = false
}
variable "datastore_type" {}
variable "node_os" {}
variable "db_username" {}
variable "db_password" {}
variable "external_db" {}
variable "external_db_version" {}
variable "db_instance_class" {}
variable "db_group_name" {}
variable "db_environment" {}
variable "engine_mode" {}
variable "install_mode" {
  default = "INSTALL_K3S_VERSION"
}
variable "k3s_channel" {
  default = "testing"
}
variable "create_lb" {
  description = "Create Network Load Balancer if set to true"
  type = bool
  default = false
}
