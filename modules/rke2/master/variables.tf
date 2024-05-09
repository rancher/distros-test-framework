variable "access_key" {}
variable "key_name" {}
variable "availability_zone" {}
variable "aws_ami" {}
variable "aws_user" {}
variable "ec2_instance_class" {}
variable "volume_size" {}
variable "iam_role" {}
variable "node_os" {}
variable "no_of_server_nodes" {}
variable "password" {
  default = "password"
}
variable "hosted_zone" {}
variable "region" {}
variable "resource_name" {}
variable "rke2_version" {}
variable "install_mode" {}
variable "install_method" {}
variable "rke2_channel" {}
variable "server_flags" {}
variable "sg_id" {}
variable "subnets" {}
variable "vpc_id" {}
variable "username" {
  default = "username"
}
variable "create_lb" {
  description = "Create Network Load Balancer if set to true"
  type = bool
}
variable "split_roles" {
  description = "When true, server nodes may be a mix of etcd, cp, and worker"
  type = bool
  default = false
}
variable "role_order" {
  description = "Comma separated order of how to bring the nodes up when split roles"
  type = string
}
variable "all_role_nodes" {}
variable "etcd_only_nodes" {
  default = 0
}
variable "etcd_cp_nodes" {
  default = 0
}
variable "etcd_worker_nodes" {
  default = 0
}
variable "cp_only_nodes" {
  default = 0
}
variable "cp_worker_nodes" {
  default = 0
}
variable "optional_files" {}
variable product {
  default = "rke2"
}
variable "enable_public_ip" {
  default = true
}
variable "enable_ipv6" {
  default = false
}

variable "datastore_type" {}
variable "external_db" {}
variable "external_db_version" {}
variable "instance_class" {}
variable "db_group_name" {}
variable "db_username" {}
variable "db_password" {}
variable "environment" {}
variable "engine_mode" {}